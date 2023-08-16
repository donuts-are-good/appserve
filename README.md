![appserve](https://github.com/donuts-are-good/appserve/assets/96031819/ca82499e-955f-4e97-9eb2-890f9e1c52d4)
![donuts-are-good's followers](https://img.shields.io/github/followers/donuts-are-good?&color=555&style=for-the-badge&label=followers) ![donuts-are-good's stars](https://img.shields.io/github/stars/donuts-are-good?affiliations=OWNER%2CCOLLABORATOR&color=555&style=for-the-badge) ![donuts-are-good's visitors](https://komarev.com/ghpvc/?username=donuts-are-good&color=555555&style=for-the-badge&label=visitors)

# appserve

appserve is a reverse proxy server in go. it routes requests based on domain to specified ports.


## usage
```$ ./appserve```

appserve reads routes from `routes.json`.
## interactive shell

upon starting, appserve provides an interactive shell with several commands:

- `list`: display all of the domain-port mappings.

- `add <domain> <port>`: add a mapping for the domain to the specified port.

- `remove <domain>`: remove the mapping for the specified domain.

- `save`: save the routes to the current routes.json file.

- `load`: load routes from the current routes.json file.

- `help`: display the help menu.

- `exit`: exit the application.

## adding routes

to add a new route via the interactive shell:

```
> add example.com 9000
```

this command routes `example.com` to port `9000`. the route is also saved to `routes.json`.

## removing routes

to remove an existing route:

```
> remove example.com
```

## configuration
### routes file

the routes file is a json array of domain-port pairs:

```
[
    {
        "domain": "example1.com",
        "port": "9000"
    },
    {
        "domain": "example2.com",
        "port": "9001"
    }
]
```

### custom routes file

to specify a custom routes file location at startup:

```
$ ./appserve -routes /path/to/your/routes.json
```


## logging

appserve logs information to the system logger (syslog). ensure you have permissions to write to the syslog.


## license

mit license 2023 donuts-are-good, for more info see license.md
