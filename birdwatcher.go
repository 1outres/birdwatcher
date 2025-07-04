package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"strings"

	"github.com/alice-lg/birdwatcher/bird"
	"github.com/alice-lg/birdwatcher/endpoints"

	"github.com/julienschmidt/httprouter"
)

//go:generate versionize
var VERSION = "2.0.0"

func isModuleEnabled(module string, modulesEnabled []string) bool {
	for _, enabled := range modulesEnabled {
		if enabled == module {
			return true
		}
	}

	return false
}

func makeRouter(config endpoints.ServerConfig) *httprouter.Router {
	whitelist := config.ModulesEnabled

	r := httprouter.New()
	if isModuleEnabled("status", whitelist) {
		r.GET("/version", endpoints.Version(VERSION))
		r.GET("/status", endpoints.Endpoint(endpoints.Status))
	}
	if isModuleEnabled("protocols", whitelist) {
		r.GET("/protocols", endpoints.Endpoint(endpoints.Protocols))
	}
	if isModuleEnabled("protocols_bgp", whitelist) {
		r.GET("/protocols/bgp", endpoints.Endpoint(endpoints.Bgp))
	}
	if isModuleEnabled("protocols_short", whitelist) {
		r.GET("/protocols/short", endpoints.Endpoint(endpoints.ProtocolsShort))
	}
	if isModuleEnabled("symbols", whitelist) {
		r.GET("/symbols", endpoints.Endpoint(endpoints.Symbols))
	}
	if isModuleEnabled("symbols_tables", whitelist) {
		r.GET("/symbols/tables", endpoints.Endpoint(endpoints.SymbolTables))
	}
	if isModuleEnabled("symbols_protocols", whitelist) {
		r.GET("/symbols/protocols", endpoints.Endpoint(endpoints.SymbolProtocols))
	}
	if isModuleEnabled("routes_protocol", whitelist) {
		r.GET("/routes/protocol/:protocol", endpoints.Endpoint(endpoints.ProtoRoutes))
	}
	if isModuleEnabled("routes_peer", whitelist) {
		r.GET("/routes/peer/:peer", endpoints.Endpoint(endpoints.PeerRoutes))
	}
	if isModuleEnabled("routes_table", whitelist) {
		r.GET("/routes/table/:table", endpoints.Endpoint(endpoints.TableRoutes))
	}
	if isModuleEnabled("routes_table_filtered", whitelist) {
		r.GET("/routes/table/:table/filtered", endpoints.Endpoint(endpoints.TableRoutesFiltered))
	}
	if isModuleEnabled("routes_table_peer", whitelist) {
		r.GET("/routes/table/:table/peer/:peer", endpoints.Endpoint(endpoints.TableAndPeerRoutes))
	}
	if isModuleEnabled("routes_count_protocol", whitelist) {
		r.GET("/routes/count/protocol/:protocol", endpoints.Endpoint(endpoints.ProtoCount))
	}
	if isModuleEnabled("routes_count_table", whitelist) {
		r.GET("/routes/count/table/:table", endpoints.Endpoint(endpoints.TableCount))
	}
	if isModuleEnabled("routes_count_primary", whitelist) {
		r.GET("/routes/count/primary/:protocol", endpoints.Endpoint(endpoints.ProtoPrimaryCount))
	}
	if isModuleEnabled("routes_filtered", whitelist) {
		r.GET("/routes/filtered/:protocol", endpoints.Endpoint(endpoints.RoutesFiltered))
	}
	if isModuleEnabled("routes_export", whitelist) {
		r.GET("/routes/export/:protocol", endpoints.Endpoint(endpoints.RoutesExport))
	}
	if isModuleEnabled("routes_noexport", whitelist) {
		r.GET("/routes/noexport/:protocol", endpoints.Endpoint(endpoints.RoutesNoExport))
	}
	if isModuleEnabled("routes_prefixed", whitelist) {
		r.GET("/routes/prefix", endpoints.Endpoint(endpoints.RoutesPrefixed))
	}
	if isModuleEnabled("route_net", whitelist) {
		r.GET("/route/net/:net", endpoints.Endpoint(endpoints.RouteNet))
		r.GET("/route/net/:net/table/:table", endpoints.Endpoint(endpoints.RouteNetTable))
	}
	if isModuleEnabled("route_net_mask", whitelist) {
		r.GET("/route/net/:net/mask/:mask", endpoints.Endpoint(endpoints.RouteNetMask))
		r.GET("/route/net/:net/mask/:mask/table/:table", endpoints.Endpoint(endpoints.RouteNetMaskTable))
	}
	if isModuleEnabled("routes_pipe_filtered_count", whitelist) {
		r.GET("/routes/pipe/filtered/count", endpoints.Endpoint(endpoints.PipeRoutesFilteredCount))
	}
	if isModuleEnabled("routes_pipe_filtered", whitelist) {
		r.GET("/routes/pipe/filtered", endpoints.Endpoint(endpoints.PipeRoutesFiltered))
	}

	return r
}

// Print service information like, listen address,
// access restrictions and configuration flags
func PrintServiceInfo(conf *Config, birdConf bird.BirdConfig) {
	// General Info
	log.Println("Starting Birdwatcher")
	log.Println("            Using:", birdConf.BirdCmd)
	log.Println("           Listen:", birdConf.Listen)
	log.Println("        Cache TTL:", birdConf.CacheTtl)

	// Endpoint Info
	if len(conf.Server.AllowFrom) == 0 {
		log.Println("        AllowFrom: ALL")
	} else {
		log.Println("        AllowFrom:", strings.Join(conf.Server.AllowFrom, ", "))
	}

	if conf.Cache.UseRedis {
		log.Println("    Caching backend: REDIS")
		log.Println("       Using server:", conf.Cache.RedisServer)
	} else {
		log.Println("    Caching backend: MEMORY")
	}

	log.Println("   ModulesEnabled:")
	for _, m := range conf.Server.ModulesEnabled {
		log.Println("       -", m)
	}
}

// MyLogger is our own log.Logger wrapper so we can customize it
type MyLogger struct {
	logger *log.Logger
}

// Write implements the Write method of io.Writer
func (m *MyLogger) Write(p []byte) (n int, err error) {
	m.logger.Print(string(p))
	return len(p), nil
}

func main() {
	// Disable timestamps for the default logger, as they are generated by the syslog implementation
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	bird6 := flag.Bool("6", false, "Use bird6 instead of bird")
	workerPoolSize := flag.Int("worker-pool-size", 8, "Number of go routines used to parse routing tables concurrently")
	configfile := flag.String("config", "/etc/birdwatcher/birdwatcher.conf", "Configuration file location")

	// Profiling
	memoryProfile := flag.String("memprofile", "", "write memory profile to this file")

	flag.Parse()

	// Start memory profiling if filename is present
	if *memoryProfile != "" {
		go startMemoryProfile(*memoryProfile)
	}

	bird.WorkerPoolSize = *workerPoolSize

	conf, err := LoadConfigs([]string{*configfile})
	if err != nil {
		log.Fatal("Loading birdwatcher configuration failed:", err)
	}

	if conf.Server.EnableTLS {
		if len(conf.Server.Crt) == 0 || len(conf.Server.Key) == 0 {
			log.Fatalln("You have enabled TLS support. Please specify 'crt' and 'key' in birdwatcher config file.")
		}
	}

	endpoints.VERSION = VERSION
	bird.InstallRateLimitReset()

	// Get config according to flags
	birdConf := conf.Bird
	if *bird6 {
		birdConf = conf.Bird6
		bird.IPVersion = "6"
	}

	PrintServiceInfo(conf, birdConf)

	// Configuration
	bird.ClientConf = birdConf
	bird.StatusConf = conf.Status
	bird.RateLimitConf.Lock()
	bird.RateLimitConf.Conf = conf.Ratelimit
	bird.RateLimitConf.Unlock()
	bird.ParserConf = conf.Parser
	bird.CacheConf = conf.Cache
	bird.InitializeCache()

	endpoints.Conf = conf.Server

	// Make server
	r := makeRouter(conf.Server)

	// Set up our own custom log.Logger without a prefix
	myquerylog := log.New(os.Stdout, "", 0)
	// Disable timestamps, as they are contained in the query log
	myquerylog.SetFlags(myquerylog.Flags() &^ (log.Ldate | log.Ltime))
	// mylogger := &MyLogger{myquerylog}

	go Housekeeping(conf.Housekeeping, !(bird.CacheConf.UseRedis)) // expire caches only for MemoryCache

	if conf.Server.EnableTLS {
		if len(conf.Server.Crt) == 0 || len(conf.Server.Key) == 0 {
			log.Fatalln("You have enabled TLS support but not specified both a .crt and a .key file in the config.")
		}
		log.Fatal(http.ListenAndServeTLS(birdConf.Listen, conf.Server.Crt, conf.Server.Key, r))
	} else {
		log.Fatal(http.ListenAndServe(birdConf.Listen, r))
	}
}
