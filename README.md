# slacker - Slack CLI to update status and presence

## Installation and setup

```
go install github.com/inkel/slacker
```

Create a configuration file (`~/.slack` by default) with the following format:

```
team-name-1 xoxp-0123456789-0987654321-201707160255-01a23b45c67d89ef0ed9cb8a01234567
team-name-2 xoxp-8623849939-8648418000-201605041419-cb8a0123456767d89ef0ed901a23b45c
```

You can create the tokens here with the [Legacy token generator](https://api.slack.com/custom-integrations/legacy-tokens).

## Usage
```
$ slacker -h
Usage of slacker:
  -away
    	Set yourself away
  -clear
    	Clears your away and custom status
  -config string
    	Which config file to use (default "~/.slack")
  -d	Enable debug mode
  -emoji string
    	Set status emoji
  -text string
    	Set status text
```

You can pass team names as additional arguments and then `slacker` will operate only in those teams, otherwise it will operate on all the teams in your configuration file.

## License

See [LICENSE](LICENSE).