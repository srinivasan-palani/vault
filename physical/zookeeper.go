package physical

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/samuel/go-zookeeper/zk"
)

// ZookeeperBackend is a physical backend that stores data at specific
// prefix within Zookeeper. It is used in production situations as
// it allows Vault to run on multiple machines in a highly-available manner.
type ZookeeperBackend struct {
	path   string
	client *zk.Conn
}

// newZookeeperBackend constructs a Zookeeper backend using the given API client
// and the prefix in the KV store.
func newZookeeperBackend(conf map[string]string) (Backend, error) {
	// Get the path in Zookeeper
	path, ok := conf["path"]
	if !ok {
		path = "vault/"
	}

	// Ensure path is suffixed but not prefixed
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Configure the client
	machines := strings.Split(conf["address"], ",")

	client, _, err := zk.Connect(machines, time.Second)
	if err != nil {
		return nil, fmt.Errorf("client setup failed: %v", err)
	}

	// Setup the backend
	c := &ZookeeperBackend{
		path:   path,
		client: client,
	}
	return c, nil
}

// zookeeper requires nodes to be there before set and get
func (c *ZookeeperBackend) ensurePath(path string) {

	nodes := strings.Split(path, "/")

	acl := zk.WorldACL(zk.PermAll)

	fullPath := ""
	for _, node := range nodes {
		if strings.TrimSpace(node) != "" {
			fullPath += "/" + node

			if exists, _, _ := c.client.Exists(fullPath); !exists {
				c.client.Create(fullPath, nil, int32(0), acl)
			}
		}
	}
}

// Put is used to insert or update an entry
func (c *ZookeeperBackend) Put(entry *Entry) error {
	defer metrics.MeasureSince([]string{"zookeeper", "put"}, time.Now())

	fullPath := c.path + entry.Key

	c.ensurePath(fullPath)

	_, err := c.client.Set(fullPath, entry.Value, 0)

	return err
}

// Get is used to fetch an entry
func (c *ZookeeperBackend) Get(key string) (*Entry, error) {
	defer metrics.MeasureSince([]string{"zookeeper", "get"}, time.Now())

	fullPath := c.path + key

	c.ensurePath(fullPath)

	value, _, err := c.client.Get(fullPath)

	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	ent := &Entry{
		Key:   key,
		Value: value,
	}
	return ent, nil
}

// Delete is used to permanently delete an entry
func (c *ZookeeperBackend) Delete(key string) error {
	defer metrics.MeasureSince([]string{"zookeeper", "delete"}, time.Now())

	fullPath := c.path + key

	exists, _, err := c.client.Exists(fullPath)

	if err != nil {
		return err
	}

	if exists {
		return c.client.Delete(fullPath, -1)
	} else {
		return nil
	}
}

// List is used ot list all the keys under a given
// prefix, up to the next prefix.
func (c *ZookeeperBackend) List(prefix string) ([]string, error) {
	defer metrics.MeasureSince([]string{"zookeeper", "list"}, time.Now())

	fullPath := strings.TrimSuffix(c.path+prefix, "/")

	c.ensurePath(fullPath)

	result, _, _ := c.client.Children(fullPath)

	children := []string{}

	for _, key := range result {
		children = append(children, key)

		nodeChildren, _, _ := c.client.Children(fullPath + "/" + key)
		if nodeChildren != nil && len(nodeChildren) > 0 {
			children = append(children, key+"/")
		}
	}

	sort.Strings(children)

	return children, nil
}

// // Lock is used for mutual exclusion based on the given key.
// func (c *ZookeeperBackend) LockWith(key, value string) (Lock, error) {
// 	// Create the lock
// 	opts := &api.LockOptions{
// 		Key:         key,
// 		Value:       []byte(value),
// 		SessionName: "Vault Lock",
// 	}
// 	lock, err := c.client.LockOpts(opts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create lock: %v", err)
// 	}
// 	cl := &ZookeeperLock{
// 		client: c.client,
// 		key:    key,
// 		lock:   lock,
// 	}
// 	return cl, nil
// }

// // DetectHostAddr is used to detect the host address by asking the Zookeeper agent
// func (c *ZookeeperBackend) DetectHostAddr() (string, error) {
// 	agent := c.client.Agent()
// 	self, err := agent.Self()
// 	if err != nil {
// 		return "", err
// 	}
// 	addr := self["Member"]["Addr"].(string)
// 	return addr, nil
// }

// ZookeeperLock is used to provide the Lock interface backed by Zookeeper
// type ZookeeperLock struct {
// 	client *zk.Conn
// 	key    string
// 	lock   *api.Lock
// }

// func (c *ZookeeperLock) Lock(stopCh <-chan struct{}) (<-chan struct{}, error) {
// 	return c.lock.Lock(stopCh)
// }

// func (c *ZookeeperLock) Unlock() error {
// 	return c.lock.Unlock()
// }

// func (c *ZookeeperLock) Value() (bool, string, error) {
// 	kv := c.client.KV()
// 	pair, _, err := kv.Get(c.key, nil)
// 	if err != nil {
// 		return false, "", err
// 	}
// 	if pair == nil {
// 		return false, "", nil
// 	}
// 	held := pair.Session != ""
// 	value := string(pair.Value)
// 	return held, value, nil
// }
