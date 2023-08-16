package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var server *http.Server

type DomainRoute struct {
	Domain string `json:"domain"`
	Port   string `json:"port"`
}

type Proxy struct {
	Port  string
	Proxy *httputil.ReverseProxy
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

	go func() {
		http.HandleFunc("/", Handler(routes, &mu))
		server = &http.Server{Addr: ":80", Handler: nil}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if strings.Contains(err.Error(), "address already in use") {
				log.Fatalf("Port 80 is already in use. Please make sure no other services are running on this port.")
			} else {
				log.Fatalf("Failed to start server: %v", err)
			}
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
			handleListCommand(routes, &mu)
		case "add":
			if len(args) != 3 {
				fmt.Println("Error: Incorrect number of arguments. Expected: add <domain> <port>")
				continue
			}
			handleAddCommand(routes, args[1], args[2], routesFile, &mu)

		case "remove":
			if len(args) != 2 {
				fmt.Println("Error: Incorrect number of arguments. Expected: remove <domain>")
				continue
			}
			handleRemoveCommand(routes, NormalizeDomain(args[1]), routesFile, &mu)
		case "save":
			handleSaveCommand(routes, routesFile)
		case "load":
			newRoutes, err := handleLoadCommand(routesFile)
			if err == nil {
				routes = newRoutes
			}
		case "help":
			handleHelpCommand()
		case "exit":
			if server != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := server.Shutdown(ctx); err != nil {
					log.Println("Server shutdown failed:", err)
				}
			}
			return

		default:
			fmt.Println("Error: Unknown command. Type 'help' for available commands.")
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdin: %v\n", err)
	}
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
		return nil, fmt.Errorf("error decoding JSON from file %s: %w", file, err)
	}

	rp := make(map[string]*Proxy)
	for _, route := range routes {
		route.Domain = NormalizeDomain(route.Domain)
		log.Println("-> route found: " + route.Domain + ":" + route.Port)
		target, err := url.Parse("http://localhost:" + route.Port)
		if err != nil {
			return nil, fmt.Errorf("error parsing URL for domain %s: %w", route.Domain, err)
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
		return fmt.Errorf("error creating file %s: %w", file, err)
	}
	defer f.Close()

	e := json.NewEncoder(f)
	return e.Encode(drs)
}

func NormalizeDomain(domain string) string {
	return strings.TrimPrefix(strings.ToLower(domain), "www.")
}

func Handler(routes map[string]*Proxy, mu *sync.RWMutex) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()

		domain := NormalizeDomain(r.Host)
		if route, found := routes[domain]; found {
			fmt.Println("hit: ", domain, routes[domain])
			route.Proxy.ServeHTTP(w, r)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}

func NewRoute(routes map[string]*Proxy, domain string, port string, mu *sync.RWMutex) error {
	domain = NormalizeDomain(domain)

	target, err := url.Parse("http://localhost:" + port)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	routes[domain] = &Proxy{
		Port:  port,
		Proxy: httputil.NewSingleHostReverseProxy(target),
	}

	return nil
}

func handleHelpCommand() {
	fmt.Printf(`

Commands:
- list: List all of the domain-port mappings.
- add <domain> <port>: Add a mapping for the domain on the specified port.
    ex: add example.com 3000
- remove <domain>: Remove a mapping for the domain.
    ex: remove example.com
- save [filepath]: Save the routes to the specified filepath or default path if not specified.
- load [filepath]: Load routes from the specified filepath or default path if not specified.
- help: Show this help.
- exit: Exit the program.

`)
}

func handleListCommand(routes map[string]*Proxy, mu *sync.RWMutex) {
	mu.RLock()
	defer mu.RUnlock()
	for domain, proxy := range routes {
		domain = NormalizeDomain(domain)
		fmt.Printf("Domain: %s, Port: %s\n", domain, proxy.Port)
	}
}

func handleAddCommand(routes map[string]*Proxy, domain, port, routesFile string, mu *sync.RWMutex) {
	if err := NewRoute(routes, domain, port, mu); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	err := SaveRoutes(routesFile, routes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Added new route for domain: %s on port: %s\n", domain, port)
	}
}

func handleRemoveCommand(routes map[string]*Proxy, domain, routesFile string, mu *sync.RWMutex) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := routes[domain]; !exists {
		fmt.Printf("Error: No such domain: %s\n", domain)
		return
	}
	delete(routes, domain)
	err := SaveRoutes(routesFile, routes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Removed route for domain: %s\n", domain)
	}
}

func handleSaveCommand(routes map[string]*Proxy, routesFile string) {
	err := SaveRoutes(routesFile, routes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Routes saved to: %s\n", routesFile)
	}
}

func handleLoadCommand(routesFile string) (map[string]*Proxy, error) {
	routes, err := LoadRoutes(routesFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Error: No such file: %s\n", routesFile)
			return nil, err
		} else {
			fmt.Printf("Error: %v\n", err)
			return nil, err
		}
	}
	fmt.Printf("Routes loaded from: %s\n", routesFile)
	return routes, nil
}
