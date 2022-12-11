# domainlookup

RDAP based tool that bulk lookups **top domain** like com, net if they are registered

[https://lookup.icann.org/en](https://lookup.icann.org/en)

## build

go build -o domainlookup main.go

## usage

### lookup by domain

domainlookup -d a.com -d b.com -c 100

### lookup by domain list

    == domains.csv ==
    a.com
    b.com

domainlookup -f domains.csv -c 100