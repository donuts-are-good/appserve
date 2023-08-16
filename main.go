package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
)

// DomainRoute represents a domain and its corresponding port
type DomainRoute struct {
	Domain string `json:"domain"`
	Port   string `json:"port"`
}

type Proxy struct {
	Port  string
	Proxy *httputil.ReverseProxy
}

func LoadRoutes(file string) (map[string]*Proxy, error) {
	log.Println("Loading routes: " + file)
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var routes []DomainRoute
	d := json.NewDecoder(f)
	if err := d.Decode(&routes); err != nil {
		return nil, err
	}

	rp := make(map[string]*Proxy)
	for _, route := range routes {
		log.Println("-> route found: " + route.Domain + ":" + route.Port)
		target, err := url.Parse("http://localhost:" + route.Port)
		if err != nil {
			log.Println("error: " + err.Error())
			return nil, err
		}
		rp[route.Domain] = &Proxy{
			Port:  route.Port,
			Proxy: httputil.NewSingleHostReverseProxy(target),
		}
	}

	return rp, nil
}

func SaveRoutes(file string, routes map[string]*Proxy) error {
	var drs []DomainRoute
	for domain, proxy := range routes {
		drs = append(drs, DomainRoute{Domain: domain, Port: proxy.Port})
	}

	log.Println("updating routes.json...")
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	e := json.NewEncoder(f)
	return e.Encode(drs)
}

func Handler(routes map[string]*Proxy, mu *sync.RWMutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()

		domain := r.Host
		if route, found := routes[domain]; found {
			fmt.Println("hit: ", domain, routes[domain])
			route.Proxy.ServeHTTP(w, r)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}

func NewRoute(routes map[string]*Proxy, domain string, port string) error {
	target, err := url.Parse("http://localhost:" + port)
	if err != nil {
		return err
	}

	routes[domain] = &Proxy{
		Port:  port,
		Proxy: httputil.NewSingleHostReverseProxy(target),
	}

	return nil
}

func showHelp() {
	fmt.Printf(`

Commands:
- list: List all of the domain-port mappings.
- add <domain> <port>: Add a mapping for the domain on the specified port.
    ex: add example.com 3000  <-- this adds a mapping for example.com to port 9000
- remove <domain>: Remove a mapping for the domain.
    ex: remove example.com  <-- this removes example.com
- help: Show this help.
- exit: Exit the program.

`)
}

func main() {
	routesFile := "routes.json"

	routes, err := LoadRoutes(routesFile)
	if err != nil {
		if os.IsNotExist(err) {
			routes = make(map[string]*Proxy)
		} else {
			log.Fatal(err)
		}
	}

	var mu sync.RWMutex

	// Start the HTTP server in a go routine so the interactive prompt can still run
	go func() {
		http.HandleFunc("/", Handler(routes, &mu))
		if err := http.ListenAndServe(":80", nil); err != nil {
			log.Println(err)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		args := strings.Fields(line)

		if len(args) == 0 {
			continue
		}

		switch args[0] {
		case "list":
			for domain, proxy := range routes {
				fmt.Printf("Domain: %s, Port: %s\n", domain, proxy.Port)
			}

		case "add":
			if len(args) != 3 {
				fmt.Println("Error: Incorrect number of arguments. Expected: add <domain> <port>")
				continue
			}
			if err := NewRoute(routes, args[1], args[2]); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			err = SaveRoutes(routesFile, routes)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Added new route for domain: %s on port: %s\n", args[1], args[2])
			}

		case "remove":
			if len(args) != 2 {
				fmt.Println("Error: Incorrect number of arguments. Expected: remove <domain>")
				continue
			}
			domain := args[1]
			if _, exists := routes[domain]; !exists {
				fmt.Printf("Error: No such domain: %s\n", domain)
				continue
			}
			delete(routes, domain)
			err = SaveRoutes(routesFile, routes)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Removed route for domain: %s\n", domain)
			}

		case "help":
			showHelp()

		case "exit":
			return

		default:
			fmt.Println("Error: Unknown command. Type 'help' for available commands.")
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdin: %v\n", err)
	}
}
