package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	fmt.Fprintln(os.Stderr)
	os.Exit(code)
}

func main() {
	var (
		away    = flag.Bool("away", false, "Set yourself away")
		online  = flag.Bool("online", false, "Set yourself online")
		emoji   = flag.String("emoji", "", "Set status emoji")
		text    = flag.String("text", "", "Set status text")
		clear   = flag.Bool("clear", false, "Clears your away and custom status")
		cfgFile = flag.String("config", defaultConfigFile(), "Which config file to use")
		debug   = flag.Bool("d", false, "Enable debug mode")

		expiresIn  = flag.Duration("expires-in", 0, "Set a relative expiration (e.g. 1h15m30s)")
		expiresAt  = flag.String("expires-at", "", "Set an absolute expiration")
		expiration = int64(0)
	)
	flag.Parse()

	if !(*clear || *away || *online || *emoji != "" || *text != "") {
		flag.Usage()
		exit(3, "")
	}

	if *clear && (*emoji != "" || *text != "") {
		exit(3, "-clear cannot be used with -away, -text or -emoji")
	}
	if *online && *away {
		exit(3, "-online and -away are mutually exclusive")
	}

	if *expiresAt != "" && *expiresIn > 0 {
		exit(4, "-expires-at and -expires-in are mutually exclusive")
	}
	if *expiresIn > 0 {
		expiration = time.Now().Add(*expiresIn).Unix()
	} else if *expiresAt != "" {
		date, err := time.Parse("2006-01-02T15:04:05", *expiresAt)
		if err != nil {
			exit(5, "cannot parse ISO8601 argument %q: %v", *expiresAt, err)

		}
		expiration = date.Unix()
	}
	if expiration > 0 && *clear {
		exit(6, "cannot use expiration when clearing the status")
	}

	tokens, err := getTokens(*cfgFile)
	if err != nil {
		exit(1, "Failure while reading configuration file: %v\n", err)
	}

	var wg sync.WaitGroup

	for team, token := range tokens {
		wg.Add(1)

		go func(team, token string) {
			defer wg.Done()

			c := client{token}

			if *debug {
				fmt.Println("Updating", team)
			}

			if *clear {
				if err := c.clearStatus(); err != nil {
					warn("Cannot clear status in %s: %v\n", team, err)
				}
			}
			if *emoji != "" || *text != "" {
				if err := c.setStatus(*emoji, *text, expiration); err != nil {
					warn("Cannot set custom status in %s: %v\n", team, err)
				}
			}

			var presence string

			if *online {
				presence = "auto"
			} else if *away {
				presence = "away"
			}

			if presence != "" {
				if err := c.setPresence(presence); err != nil {
					warn("Cannot set presence to %s in %s: %v\n", presence, team, err)
				}
			}
		}(team, token)
	}

	wg.Wait()
}

func getTokens(file string) (map[string]string, error) {
	fp, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	var (
		tokens = make(map[string]string)
		teams  = flag.Args()
	)

	includeTeam := func(team string) bool {
		if len(teams) == 0 {
			return true
		}
		for _, t := range teams {
			if t == team {
				return true
			}
		}
		return false
	}

	scanner := bufio.NewScanner(fp)

	var nline int

	for scanner.Scan() {
		nline++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.IndexRune(line, '#') == 0 {
			continue
		}

		var team, token string

		_, err := fmt.Sscan(line, &team, &token)
		if err != nil {
			warn("error when reading line %d: %q => %v\n", nline, line, err)
			continue
		}

		if includeTeam(team) {
			tokens[team] = token
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens found for teams: %v", strings.Join(teams, ", "))
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

func (c client) setPresence(presence string) error {
	return c.do("users.setPresence", url.Values{"presence": []string{presence}})
}

func (c client) setStatus(emoji, text string, expiration int64) error {
	profile := fmt.Sprintf(`{"status_text":%q,"status_emoji":%q`, text, emoji)
	if expiration > 0 {
		profile = fmt.Sprintf(`%s,"status_expiration":%d`, profile, expiration)
	}
	profile = profile + "}"
	return c.do("users.profile.set", url.Values{"profile": []string{profile}})
}
func (c client) clearStatus() error { return c.setStatus("", "", 0) }

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
