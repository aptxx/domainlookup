// Package main
// domainlookup is a command tool to bulk looks up domains using RDAP database
// https://lookup.icann.org/en/lookup
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

const (
	defaultConcurrency = 256
)

// array flag. e.g. -d a.com -d b.com
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(s string) error {
	*i = append(*i, s)
	return nil
}

// flags
var (
	fConcurrency int
	fDomain      arrayFlags
	fFile        string
)

func init() {
	flag.IntVar(&fConcurrency, "c", defaultConcurrency, "Max QPS lookups RDAP. Default is 256")
	flag.Var(&fDomain, "d", "Domain to check")
	flag.StringVar(&fFile, "f", "", "Domains file to check, one domain per line")
}

// response example
// {
//   "description": "RDAP bootstrap file for Domain Name System registrations",
//   "publication": "2022-12-08T18:00:02Z",
//   "services": [
//     [
//       [
//         "uz"
//       ],
//       [
//         "http://cctld.uz:9000/"
//       ]
//     ]
//   ]
// }
const rdapDNSURL = "https://data.iana.org/rdap/dns.json"

// RdapDNS struct from icann response
type RdapDNS struct {
	Description string           `json:"description"`
	Publication string           `json:"publication"`
	Services    []RdapDNSservice `json:"services"`
}

type RdapDNSservice [][]string

// return top domain -> rdap urls
func (dns *RdapDNS) LookupMap() (m map[string][]string, err error) {
	if dns == nil || len(dns.Services) == 0 {
		return nil, errors.New("rdap services is empty")
	}

	m = make(map[string][]string)
	for _, service := range dns.Services {
		if len(service) != 2 {
			return nil, fmt.Errorf("service is not a tuple. service %+v", service)
		}
		for _, topdomain := range service[0] {
			m[topdomain] = service[1]
		}
	}
	return m, nil
}

func rdapDNSInfo(dnsURL string) (dns *RdapDNS, err error) {
	resp, err := http.Get(dnsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	dns = &RdapDNS{}
	err = json.Unmarshal(body, &dns)
	return
}

// domainlookup result
type DomainLookupResult struct {
	Domain  string
	Message string
	Result  *RdapLookupResult
}

// RdapLookupResult of protocl
type RdapLookupResult struct {
}

type LookupWorker struct {
	unchecked <-chan string

	rdapLookupMap map[string][]string

	concurrencies chan struct{}

	concurrencyLimit int

	Result chan *DomainLookupResult
}

func (worker *LookupWorker) topdomain(domain string) string {
	if domain == "" {
		return ""
	}
	arr := strings.Split(domain, ".")
	return arr[len(arr)-1]
}

func (worker *LookupWorker) rdapLookupURL(rdap string, domain string) string {
	return fmt.Sprintf("%s/domain/%s", rdap, domain)
}

// looks like verisign response 404 means domain is not registered. so we
// only to check the response http status
// NOTE: we ONLY support top domain like com, net at this moment
func (worker *LookupWorker) queryRdap(rdap, domain string) (resp *http.Response, err error) {
	query := worker.rdapLookupURL(rdap, domain)
	resp, err = http.Get(query)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	return
}

func (worker *LookupWorker) Start() {
	wg := sync.WaitGroup{}

	for domain := range worker.unchecked {
		wg.Add(1)
		worker.concurrencies <- struct{}{}

		go func(domain string) {
			defer func() {
				<-worker.concurrencies
				wg.Done()
			}()

			apis, ok := worker.rdapLookupMap[worker.topdomain(domain)]
			if !ok || len(apis) == 0 {
				worker.Result <- &DomainLookupResult{
					Domain:  domain,
					Message: "No RDAP server found",
				}
				return
			}

			resp, err := worker.queryRdap(apis[0], domain)
			if err != nil {
				worker.Result <- &DomainLookupResult{
					Domain:  domain,
					Message: err.Error(),
				}
				return
			}

			statusCode := resp.StatusCode
			message := ""
			switch {
			case statusCode >= 200 && statusCode < 300:
				message = "Registered"
			case statusCode == 404:
				message = "Unregistered"
			case statusCode >= 500:
				message = "RDAP server error"
			default:
				message = "Unknown error"
			}
			worker.Result <- &DomainLookupResult{
				Domain:  domain,
				Message: message,
			}
		}(domain)
	}

	wg.Wait()
	close(worker.Result)
}

func main() {
	flag.Parse()

	if len(fDomain) == 0 && fFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	rdapDNS, err := rdapDNSInfo(rdapDNSURL)
	if err != nil {
		log.Fatal(err)
	}
	rdapMap, err := rdapDNS.LookupMap()
	if err != nil {
		log.Fatal(err)
	}

	unchecked := make(chan string)
	lookupWorker := &LookupWorker{
		unchecked:        unchecked,
		rdapLookupMap:    rdapMap,
		concurrencies:    make(chan struct{}, fConcurrency),
		concurrencyLimit: fConcurrency,
		Result:           make(chan *DomainLookupResult),
	}

	go lookupWorker.Start()

	go func() {
		for _, domain := range fDomain {
			unchecked <- domain
		}
		if fFile != "" {
			file, err := os.Open(fFile)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				unchecked <- scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
		}
		close(unchecked)
	}()

	for result := range lookupWorker.Result {
		fmt.Printf("%s,%s\n", result.Domain, result.Message)
	}
}
