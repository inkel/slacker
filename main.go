package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func warn(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func exit(code int, format string, args ...interface{}) {
	warn(format, args...)
	os.Exit(code)
}

func main() {
	var (
		away    = flag.Bool("away", false, "Set yourself away")
		emoji   = flag.String("emoji", "", "Set status emoji")
		text    = flag.String("text", "", "Set status text")
		clear   = flag.Bool("clear", false, "Clears your away and custom status")
		cfgFile = flag.String("config", defaultConfigFile(), "Which config file to use")
		debug   = flag.Bool("d", false, "Enable debug mode")
	)
	flag.Parse()

	if !(*clear || *away || *emoji != "" || *text != "") {
		flag.Usage()
		exit(3, "")
	}

	if *clear && (*away || *emoji != "" || *text != "") {
		exit(3, "-clear cannot be used with -away, -text or -emoji")
	}

	tokens, err := readConfig(*cfgFile)
	if err != nil {
		exit(1, "Failure to read configuration file: %v\n", err)
	}

	var wg sync.WaitGroup

	for team, token := range tokens {
		wg.Add(1)

		go func(team, token string) {
			defer wg.Done()

			if skipTeam(team) {
				if *debug {
					fmt.Println("Skipping", team)
				}
				return
			}

			c := client{token}

			if *debug {
				fmt.Println("Updating", team)
			}

			if *clear {
				if err := c.clearAway(); err != nil {
					warn("Cannot clear away status in %s: %v\n", team, err)
				}
				if err := c.clearStatus(); err != nil {
					warn("Cannot clear status in %s: %v\n", team, err)
				}
			}

			if *away {
				if err := c.setAway(); err != nil {
					warn("Cannot set presence to away in %s: %v\n", team, err)
				}
			}
			if *emoji != "" || *text != "" {
				if err := c.setStatus(*emoji, *text); err != nil {
					warn("Cannot set custom status in %s: %v\n", team, err)
				}
			}
		}(team, token)
	}

	wg.Wait()
}

func readConfig(file string) (map[string]string, error) {
	fp, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	tokens := make(map[string]string)

	for {
		var team, token string
		_, err := fmt.Fscanln(fp, &team, &token)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		tokens[team] = token
	}

	return tokens, nil
}

func defaultConfigFile() string {
	u, err := user.Current()
	if err != nil {
		exit(2, "Couldn't get current user: %v\n", err)
	}

	return filepath.Join(u.HomeDir, ".slack")
}

type client struct {
	Token string
}

func (c client) setAway() error {
	return c.do("users.setPresence", url.Values{"presence": []string{"away"}})
}
func (c client) clearAway() error {
	return c.do("users.setPresence", url.Values{"presence": []string{"auto"}})
}

func (c client) setStatus(emoji, text string) error {
	profile := fmt.Sprintf(`{"status_text":%q,"status_emoji":%q}`, text, emoji)
	return c.do("users.profile.set", url.Values{"profile": []string{profile}})
}
func (c client) clearStatus() error { return c.setStatus("", "") }

func (c client) do(method string, values url.Values) error {
	values.Set("token", c.Token)

	body := strings.NewReader(values.Encode())

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("https://slack.com/api/%s", method), body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Charset", "utf-8")
	req.Header.Set("User-Agent", "slacker/0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	req = req.WithContext(ctx)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var sres struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(data, &sres); err != nil {
		return err
	}

	if !sres.Ok {
		return errors.New(sres.Error)
	}

	return nil
}

func skipTeam(team string) bool {
	if len(flag.Args()) == 0 {
		return false
	}
	for _, t := range flag.Args() {
		if t == team {
			return false
		}
	}
	return true
}
