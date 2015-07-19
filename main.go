package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-etcd/etcd"
	docker "github.com/fsouza/go-dockerclient"
)

var (
	etcdMachines = strings.Split(os.Getenv("SKYDOCK_MACHINES"), ",") // etcd api urls
	etcdPrefix   = os.Getenv("SKYDOCK_PREFIX")                       // etcd keyspace prefix (e.g. /skydns
	skydnsDomain = os.Getenv("SKYDOCK_DOMAIN")                       // skydns domain (e.g. skydns.local)
	envName      = os.Getenv("SKYDOCK_ENV")                          // env name (e.g. dev for dev.skydns.local)
	etcdTTL      = os.Getenv("SKYDOCK_TTL")                          // etcd ttl for skydns keys
	etcdBeat     = os.Getenv("SKYDOCK_BEAT")                         // heartbeat interval

	dockerclient *docker.Client
	skydns       *SkyDNS
)

// add existing containers to dns
func registerContainers() (int, error) {
	containers, err := dockerclient.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return 0, fmt.Errorf("listing containers: %v", err)
	}

	count := 0
	for _, c := range containers {
		ci, err := dockerclient.InspectContainer(c.ID)
		if err != nil {
			return count, fmt.Errorf("inspecting container %q: %v", c.ID, err)
		}

		if err := skydns.Add(image2service(c.Image, false), c.ID[:10], &Service{Host: ci.NetworkSettings.IPAddress}); err != nil {
			return count, fmt.Errorf("adding skydns entry: %v", err)
		} else {
			count++
		}
	}

	return count, nil
}

// remove existing containers to dns
func unregisterContainers() (int, error) {
	containers, err := dockerclient.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return 0, fmt.Errorf("listing containers: %v", err)
	}

	count := 0
	for _, c := range containers {
		if err := skydns.Delete(image2service(c.Image, false), c.ID[:10]); err != nil {
			return count, fmt.Errorf("adding skydns entry: %v", err)
		} else {
			count++
		}
	}

	return count, nil
}

func main() {
	flag.Parse()

	if len(etcdMachines) == 0 || (len(etcdMachines) == 1 && etcdMachines[0] == "") {
		log.Fatal("no etcd endpoints given; set SKYDOCK_MACHINES=http://127.0.0.1:2379")
	}

	if etcdPrefix == "" {
		etcdPrefix = "/skydns"
		log.Printf("warning: using default SKYDOCK_PREFIX=%s", etcdPrefix)
	}

	if skydnsDomain == "" {
		skydnsDomain = "skydns.local"
		log.Printf("warning: using default SKYDOCK_DOMAIN=%s", skydnsDomain)
	}

	if envName == "" {
		envName = "dev"
		log.Printf("warning: using default SKYDOCK_ENV=%s", envName)
	}

	ttl := 0
	if etcdTTL == "" {
		ttl = 30
		log.Printf("warning: using default SKYDOCK_TTL=%d", ttl)
	} else {
		t, err := strconv.Atoi(etcdTTL)
		if err != nil {
			log.Fatal("invalid SKYDOCK_TTL: %v", err)
		}
		ttl = t
	}

	beat := 0
	if etcdBeat == "" {
		beat = ttl - (ttl / 4)
		log.Printf("warning: using default SKYDOCK_BEAT=%d", beat)
	} else {
		b, err := strconv.Atoi(etcdBeat)
		if err != nil {
			log.Fatal("invalid SKYDOCK_BEAT: %v", err)
		}
		beat = b
	}

	skydns = NewSkyDNS(etcdMachines, etcdPrefix, envName+"."+skydnsDomain, ttl, beat)

	dc, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("error connecting to docker: %v", err)
	}

	dockerclient = dc

	log.Printf("registering containers...")
	n, err := registerContainers()
	if err != nil {
		log.Fatalf("error registering existing containers: %v", err)
	}
	log.Printf("registered %d containers", n)

	evt := make(chan *docker.APIEvents)
	err = dockerclient.AddEventListener(evt)
	if err != nil {
		log.Fatalf("error setting up docker event listener: %v", err)
	}

	note := make(chan os.Signal, 1)
	signal.Notify(note, os.Interrupt, os.Kill)

	for {
		select {
		case <-note:
			log.Printf("got note, exiting")
			dockerclient.RemoveEventListener(evt)

			n, err := unregisterContainers()
			if err != nil {
				log.Printf("error unregistering containers: %v", err)
			}
			log.Printf("unregistered %d containers", n)
			return
		case e := <-evt:
			switch e.Status {
			case "die", "kill", "stop":
				ci, err := dockerclient.InspectContainer(e.ID)
				if err != nil {
					log.Printf("docker.InspectContainer: %v", err)
					continue
				}
				if err := skydns.Delete(image2service(ci.Config.Image, false), ci.ID[:10]); err != nil {
					log.Printf("skydns.Delete: %v", err)
					continue
				}
			case "start", "restart":
				ci, err := dockerclient.InspectContainer(e.ID)
				if err != nil {
					log.Printf("docker.InspectContainer: %v", err)
					continue
				}

				svc := image2service(ci.Config.Image, false)
				inst := ci.ID[:10]
				rec := &Service{Host: ci.NetworkSettings.IPAddress}

				if err := skydns.Add(svc, inst, rec); err != nil {
					log.Printf("skydns.Add: %v", err)
					continue
				}
			}
		}
	}
}

// convert docker image name to dns endpoint.
// if owner is true the img "mischief/foo" returns "mischief-foo". otherwise, it will
// return "foo"
func image2service(img string, owner bool) string {
	spl := strings.Split(img, ":")
	if len(spl) > 0 {
		img = spl[0]
	}

	spl = strings.Split(img, "/")

	switch len(spl) {
	default:
		panic("bad len")
	case 1:
		return spl[0]
	case 2:
		if !owner {
			return spl[1]
		}
		return spl[0] + "-" + spl[1]
	case 3:
		if !owner {
			return spl[2]
		}
		return spl[1] + "-" + spl[2]
	}
}

type Service struct {
	Host     string `json:",omitempty"`
	Port     int    `json:",omitempty"`
	Priority int    `json:",omitempty"`
	Weight   int    `json:",omitempty"`
	Text     string `json:",omitempty"`
}

type SkyDNS struct {
	client *etcd.Client

	machines []string
	prefix   string
	domain   string
	ttl      uint64
	beat     uint64

	heartsmu sync.Mutex
	hearts   map[string]struct{}
}

func NewSkyDNS(machines []string, etcdprefix, domain string, ttl, beat int) *SkyDNS {
	s := &SkyDNS{
		client:   etcd.NewClient(machines),
		machines: machines,
		prefix:   etcdprefix,
		domain:   domain,
		ttl:      uint64(ttl),
		beat:     uint64(beat),
		hearts:   make(map[string]struct{}),
	}

	return s
}

// convert "redis", "a1b2c3d4e5" to "/skydns/local/skydns/dev/redis/a1b2c3d4e5"
func (s *SkyDNS) srv2key(service, instance string) string {
	key := s.prefix
	// build domain key
	spl := strings.Split(s.domain, ".")
	for i := len(spl); i > 0; i-- {
		key += "/" + spl[i-1]
	}

	// build service key
	spl = strings.Split(service, ".")
	for i := len(spl); i > 0; i-- {
		key += "/" + spl[i-1]
	}

	// build instance key
	key += "/" + instance
	return key
}

// getting deleted will cancel us
func (s *SkyDNS) heartbeat(service, instance string, srv *Service) {
	key := s.srv2key(service, instance)

	s.heartsmu.Lock()
	if _, ok := s.hearts[key]; ok {
		s.heartsmu.Unlock()
		return
	}

	s.hearts[key] = struct{}{}
	s.heartsmu.Unlock()

	defer func() {
		s.heartsmu.Lock()
		delete(s.hearts, key)
		s.heartsmu.Unlock()
	}()

	for {
		if err := s.Update(service, instance, srv); err != nil {
			log.Printf("skydns.heartbeat %q: %v", key, err)
			break
		}

		time.Sleep(time.Second * time.Duration(s.beat))
	}
}

func (s *SkyDNS) Add(service, instance string, srv *Service) error {
	b, _ := json.Marshal(srv)
	key := s.srv2key(service, instance)

	_, err := s.client.Set(key, string(b), s.ttl)

	if err == nil {
		go s.heartbeat(service, instance, srv)
	}
	return err
}

func (s *SkyDNS) Update(service, instance string, srv *Service) error {
	b, _ := json.Marshal(srv)
	key := s.srv2key(service, instance)

	_, err := s.client.Update(key, string(b), s.ttl)
	return err
}

func (s *SkyDNS) Delete(service, instance string) error {
	key := s.srv2key(service, instance)
	_, err := s.client.Delete(key, false)
	return err
}
