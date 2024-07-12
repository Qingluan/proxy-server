package geo

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"gitee.com/dark.H/ProxyZ/asset"
	"gitee.com/dark.H/gs"
	"github.com/ip2location/ip2location-go/v9"
)

var (
	COUNTRY_DB_BUF = ""
	iplock         = sync.RWMutex{}
	ipmark         = make(map[string]string)
)

type Geo struct {
	DOMAIN  string
	IP      string
	Country string
	City    string
}

func (g *Geo) Str() gs.Str {
	return gs.Str("(%s/%s)[%s/%s]").F(g.IP, g.DOMAIN, g.Country, g.City)
}

func (g *Geo) InCN() bool {
	return g.Country == "China"
}

func (g *Geo) InUSA() bool {
	return g.Country == "United States of America"
}

func InitDB() {
	tmpfle := gs.TMP.PathJoin("geo.db")
	if !tmpfle.IsExists() {
		buf, err := asset.Asset("Resources/IP2LOCATION-LITE-DB1.BIN")
		if err != nil {
			gs.Str(err.Error()).Color("r").Println()
		}

		os.WriteFile(tmpfle.Str(), buf, 0755)

	}
	COUNTRY_DB_BUF = tmpfle.Str()
}

func CountryCode(ip string) string {
	iplock.Lock()
	isCN, ok := ipmark[ip]
	iplock.Unlock()
	if ok {
		return isCN
	} else {
		ges := IP2GEO(ip)
		t := ""
		if len(ges) > 0 {
			t = ges[0].Country
			iplock.Lock()
			ipmark[ip] = t
			iplock.Unlock()
		}
		return t
	}
}

func GetCountry(ip string) string {
	if COUNTRY_DB_BUF == "" {
		InitDB()
	}
	db, err := ip2location.OpenDB(COUNTRY_DB_BUF)
	if err != nil {
		return "no db :" + err.Error()
	}
	defer db.Close()
	results, err := db.Get_country_long(ip)
	if err != nil {
		return "exists db :" + err.Error()
	}
	return results.Country_long
}

func IP2GEO(ip ...string) (points gs.List[*Geo]) {

	if COUNTRY_DB_BUF == "" {
		InitDB()
	}
	db, err := ip2location.OpenDB(COUNTRY_DB_BUF)
	if err != nil {

		path := gs.Str("~").ExpandUser().PathJoin(".cache", "geodatabase", "IP2LOCATION-LITE-DB3.BIN.2024").Str()
		db, err = ip2location.OpenDB(path)
		if err != nil {
			fmt.Print(err)
			return
		}
	}
	defer db.Close()

	for _, i := range ip {
		results, err := db.Get_all(i)
		if err != nil {
			gs.Str(i + " / " + err.Error()).Color("r").Println("GEO IP")
			return nil
		}
		points = points.Add(&Geo{
			IP:      i,
			City:    results.City,
			Country: results.Country_long,
		})
	}
	return
}

func Host2GEO(hosts ...string) (points gs.List[*Geo]) {
	path := "/.cache/geodb"
	db, err := ip2location.OpenDB(path)
	if err != nil {
		path = gs.Str("~").ExpandUser().PathJoin(".cache", "geodatabase", "IP2LOCATION-LITE-DB3.BIN").Str()
		db, err = ip2location.OpenDB(path)
		if err != nil {
			fmt.Print(err)
			return
		}

	}
	defer db.Close()
	r := &net.Resolver{}

	for _, i := range hosts {
		ct, _ := context.WithTimeout(context.Background(), 700*time.Millisecond)
		addrs, err := r.LookupHost(ct, i)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			results, err := db.Get_all(a)
			if err != nil {
				return nil
			}
			points = points.Add(&Geo{
				DOMAIN:  i,
				IP:      a,
				City:    results.City,
				Country: results.Country_long,
			})
			break
		}
	}
	return
}
