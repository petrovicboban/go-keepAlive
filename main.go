package main

import (
	"github.com/samuel/go-zookeeper/zk"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strings"
	"time"
	"net"
	"log"
	"flag"
	"bytes"
)

var (
	mode, agentName, zkList   string
        zkConn			  *zk.Conn
        acl			  []zk.ACL
)

const (
	flags_const = int32(0)
	flags_ephem = int32(zk.FlagEphemeral)
)

type packet struct {
	service string
	endpoint string
	member string
	contents string
}

type service struct  {
	Endpoints	[]string `yaml:"endpoints"`
	Name		string   `yaml:"name"`
}

type config struct {
	Services []service `yaml:"services"`
}



func init() {

        hostname, _ := os.Hostname()

	flag.StringVar(&mode, "mode", "agent", "Running mode. Can be agent or master. Default is agent.")
	flag.StringVar(&agentName, "agentName", hostname, "Agent name. Default is hostname.")
	flag.StringVar(&zkList, "zk", "localhost", "Zookeepers list, separated by comma. Default is localhost.")
	flag.Parse()

        zkConn = connect()
	acl = zk.WorldACL(zk.PermAll)
        initTree()

	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
	log.SetOutput(os.Stdout)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func connect() *zk.Conn {
	zks := strings.Split(zkList, ",")
	conn, _, err := zk.Connect(zks, 2 * time.Second)
	must(err)
	return conn
}

func createIfNotExists(path string, flags int32) (string, error){
        exists, _, err := zkConn.Exists(path)
        
        if !exists {
		path, err = zkConn.Create(path, []byte(""), flags, acl)
	}
        return path, err
}

func initTree() {
	createIfNotExists("/go-keepAlive", flags_const)
	createIfNotExists("/go-keepAlive/services", flags_const)
	createIfNotExists("/go-keepAlive/agents", flags_const)
}

func difference(slice1 []string, slice2 []string) []string {
    var diff []string

    // Loop two times, first to find slice1 strings not in slice2,
    // second loop to find slice2 strings not in slice1
    for i := 0; i < 2; i++ {
        for _, s1 := range slice1 {
            found := false
            for _, s2 := range slice2 {
                if s1 == s2 {
                    found = true
                    break
                }
            }
            // String not found. We add it to return slice
            if !found {
                diff = append(diff, s1)
            }
        }
        // Swap the slices, only if it was the first loop
        if i == 0 {
            slice1, slice2 = slice2, slice1
        }
    }

    return diff
}


func agent() {
	defer zkConn.Close()

	ch := make(chan bool, 1)
       
	services, _, err := zkConn.Children("/go-keepAlive/services")
	must(err)

        for _, service := range services {
                log.Println("Service " + service + " detected in config at zookeeper(s)")

		endpoints, _, err := zkConn.Children("/go-keepAlive/services/" + service)
        	for _, endpoint := range endpoints {
			log.Println("Service " + service + ": endpoint " + endpoint + " detected in config at zookeeper(s)")

		        _, err = createIfNotExists("/go-keepAlive/services/" + service + "/" + endpoint + "/" + agentName, flags_ephem)
			must(err)

			go func(service, endpoint string) {
				ok := 0
				nok := 0

				_, err := createIfNotExists("/go-keepAlive/agents/" + agentName, flags_ephem)
				must(err)
				for {
 					connection, err := net.DialTimeout("tcp", net.JoinHostPort(endpoint, "80"), 1000 * time.Millisecond)
					if err != nil {
						log.Printf("%s/%s: %v", service, endpoint, err)
						nok++
  						if nok == 3 {
							_, err = zkConn.Set("/go-keepAlive/services/" + service + "/" + endpoint + "/" + agentName, []byte("false"), -1)
							must(err)
							ok = 0
						}
						if nok > 3 {
							nok = 3
						}
					} else {
						log.Println(connection.RemoteAddr().String() + " established")
 						ok++
        	                	        if ok == 2 { 
							_, err = zkConn.Set("/go-keepAlive/services/" + service + "/" + endpoint + "/" + agentName, []byte("true"), -1)
							must(err)
							nok = 0
						}
					        connection.Close()
						if ok > 2 {
							ok = 2
						}
					}
					time.Sleep(time.Second * 2)
				}
			} (service, endpoint)
		}
	}
	_ = <-ch
}

func master() {
	defer zkConn.Close()

        oldSnapshot := []string{}
	
	configFile := "./config.yml"

	yamlFile, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("yamlFile.Get err   #%v ", err)
	}

	config := config{}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	log.Println(config)

	snapshots := make(chan []string)
	member_content := make(chan packet)
	errors := make(chan error)
	ch := make(chan bool, 1)

        for _, service := range config.Services {
		log.Println("Service " + service.Name + " detected")
		for _, endpoint := range service.Endpoints {
			log.Println("Service " + service.Name + ": endpoint " + endpoint + " detected")
		        _, err := zkConn.Create("/go-keepAlive/services/" + service.Name, []byte(""), flags_const, acl)
                        if err != nil {
				log.Println(err)
			}
		        _, err = zkConn.Create("/go-keepAlive/services/" + service.Name + "/" + endpoint, []byte(""), flags_const, acl)
                        if err != nil {
				log.Println(err)
			}
		}
	}
	go func() {
		for {
			select {
			case snapshot := <-snapshots:
				if len(snapshot) == 0 {
					log.Printf("Agents list is empty")
				} else {
					log.Printf("Agents list is changed: %v", snapshot)
				}
				if len(snapshot) > len(oldSnapshot) {
				        services, _, err := zkConn.Children("/go-keepAlive/services")
        	                        must(err)
 					for _, service := range services {
                				endpoints, _, err := zkConn.Children("/go-keepAlive/services/" + service)
						must(err)
			                	for _, endpoint := range endpoints {
							for _, member := range difference(snapshot, oldSnapshot) {
								go func(service, endpoint, member string) {
									log.Println("Creating watcher for " + service + "/" + endpoint + "/" + member)
									for {
										contents, _, events, err := zkConn.GetW("/go-keepAlive/services/" + service + "/" + endpoint + "/" + member )
										if err == zk.ErrNoNode {
											log.Println("Agent " + member + " dissapeared")
											break
										} 
										if len(contents) > 0 {
											member_content <- packet{service, endpoint, member, string(contents)}
										}
										evt := <-events
			                       					if evt.Err != nil {
          				              			        	errors <- evt.Err
							                                return
							                        }
									}
									log.Println("Removing watcher for " + service + "/" + endpoint + "/" + member)
								}(service, endpoint, member)
							}
						}
					}
				}
				oldSnapshot = snapshot
			case err := <-errors:
				panic(err)
			}
		}
	}()
	go func() {
		for {
			select {
			case contents := <-member_content:
				log.Printf("/go-keepAlive/services/%s/%s/%s reported: %s", contents.service, contents.endpoint, contents.member, contents.contents)
				agents, _, err := zkConn.Children("/go-keepAlive/services/" + contents.service + "/" + contents.endpoint)
				must(err)
				count := len(agents)
				i := 0
				for _, agent := range agents {
					data, _, err := zkConn.Get("/go-keepAlive/services/" + contents.service + "/" + contents.endpoint + "/" + agent)					
					must(err)
					if bytes.Equal(data, []byte("true")) {
						i++
					}
				}
				log.Printf("Checks on %s/%s: %d/%d agents reported healthy", contents.service, contents.endpoint, i, count)
				data, _, err := zkConn.Get("/go-keepAlive/services/" + contents.service)
				if float64(i)/float64(count) > 0.5 {
					if !bytes.Contains(data, []byte(contents.endpoint)) {
						data = bytes.TrimSpace(append(data, " " + contents.endpoint...))
						log.Printf("Adding endpoint %s to service %s contents", contents.endpoint, contents.service)
        	                                 _, err = zkConn.Set("/go-keepAlive/services/" + contents.service, data, -1) 
					}
				} else {
					if bytes.Contains(data, []byte(contents.endpoint)) {
						data = bytes.Replace(data, []byte(contents.endpoint), []byte(""), -1)	
						data = bytes.TrimSpace(bytes.Replace(data, []byte("  "), []byte(" "), -1))	
						log.Printf("Removing endpoint %s from service %s contents", contents.endpoint, contents.service)
        	                                 _, err = zkConn.Set("/go-keepAlive/services/" + contents.service, data, -1) 
					}
				}
			}
		}
	}()

	go func() {
		for {
			snapshot, _, events, err := zkConn.ChildrenW("/go-keepAlive/agents")
			if err != nil {
				errors <- err
				return
			}
			snapshots <- snapshot
			evt := <-events
			if evt.Err != nil {
				errors <- evt.Err
				return
			}
		}
	}()        

	_ = <-ch
}

func main() {

	switch mode {
	case "agent":
		agent()
	case "master":
		master()
	}
}
