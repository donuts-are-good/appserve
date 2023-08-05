package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
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
			fmt.Println("hit: ", route)
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

func main() {
	newDomain := flag.String("new", "", "Add new domain")
	newPort := flag.String("port", "", "Port for new domain")
	routesFile := flag.String("routes", "routes.json", "Routes file")
	flag.Parse()

	routes, err := LoadRoutes(*routesFile)
	if err != nil {
		if os.IsNotExist(err) {
			routes = make(map[string]*Proxy)
		} else {
			log.Fatal(err)
		}
	}

	var mu sync.RWMutex

	log.Println("routes loaded!")
	if *newDomain != "" && *newPort != "" {
		if err := NewRoute(routes, *newDomain, *newPort); err != nil {
			log.Fatal(err)
		}
		err = SaveRoutes(*routesFile, routes)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("added new route")
		return
	}

	http.HandleFunc("/", Handler(routes, &mu))
	if err := http.ListenAndServe(":80", nil); err != nil {
		log.Println(err)
	}
}
