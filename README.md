![appserve](https://github.com/donuts-are-good/appserve/assets/96031819/ca82499e-955f-4e97-9eb2-890f9e1c52d4)
![donuts-are-good's followers](https://img.shields.io/github/followers/donuts-are-good?&color=555&style=for-the-badge&label=followers) ![donuts-are-good's stars](https://img.shields.io/github/stars/donuts-are-good?affiliations=OWNER%2CCOLLABORATOR&color=555&style=for-the-badge) ![donuts-are-good's visitors](https://komarev.com/ghpvc/?username=donuts-are-good&color=555555&style=for-the-badge&label=visitors)

# appserve

appserve is a reverse proxy server in go. it routes requests based on domain to specified ports.


## usage
```$ ./appserve```

appserve reads routes from `routes.json`.

### adding routes

to add a new route:

```$ ./appserve -new example.com -port 9000```

this command routes example.com to port 9000. the route is saved to routes.json.
configuration
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

specify a custom routes file:

```
$ ./appserve -routes /path/to/your/routes.json
```

## license

mit license 2023 donuts-are-good, for more info see license.md
