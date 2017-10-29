package agentman

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testutil"
	"math"
	"net/http"
	"strings"
	"sync"
)

// TestInstance represents a single instance of a consul test server and its client.  May be alone or in a cluster.
type TestInstance struct {
	m sync.Mutex

	name string

	server *testutil.TestServer
	client *api.Client
}

// NewTestInstance will attempt to create a new consul test server and api client
func NewTestInstance(name string, cb testutil.ServerConfigCallback) (*TestInstance, error) {
	var err error
	s := &TestInstance{
		name: name,
	}

	s.server, err = testutil.NewTestServerConfig(cb)
	if err != nil {
		return nil, err
	}

	apiConf := api.DefaultConfig()
	apiConf.Address = s.server.HTTPAddr
	s.client, err = api.NewClient(apiConf)
	if err != nil {
		s.server.Stop()
		return nil, err
	}

	return s, nil
}

func (ti *TestInstance) Name() string {
	return ti.name
}

func (ti *TestInstance) HTTPAddr() string {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.HTTPAddr
}

func (ti *TestInstance) HTTPSAddr() string {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.HTTPSAddr
}

func (ti *TestInstance) LANAddr() string {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.LANAddr
}

func (ti *TestInstance) WANAddr() string {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.WANAddr
}

func (ti *TestInstance) HTTPClient() *http.Client {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.HTTPClient
}

func (ti *TestInstance) APIClient() *api.Client {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.client
}

// Config returns pointer to the underlying test server config.  Modify at your own risk.
func (ti *TestInstance) Config() *testutil.TestServerConfig {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		panic(fmt.Sprintf("Instance %s is defunct", ti.name))
	}
	return ti.server.Config
}

// Stop attempts to stop the underlying test server and nils about both the server and the client.  This instance
// is considered defunct after this action, and all further interaction will cause a panic.
func (ti *TestInstance) Stop() error {
	ti.m.Lock()
	defer ti.m.Unlock()
	if ti.server == nil {
		return nil
	}

	err := ti.server.Stop()
	ti.server = nil
	ti.client = nil
	return err
}

func (ti *TestInstance) Stopped() bool {
	ti.m.Lock()
	defer ti.m.Unlock()
	return ti.server == nil
}

type (
	// ClusterServerConfigCallback is a small wrapper around testutil.ServerConfigCallback that adds scope
	ClusterServerConfigCallback = func(name string, num uint8, conf *testutil.TestServerConfig)

	// TestCluster represents 2 or more agents running as a cluster
	TestCluster struct {
		m sync.Mutex

		name string

		size      uint8
		instances []*TestInstance
		stopped   bool
	}
)

var DefaultClusterServerConfigCallback ClusterServerConfigCallback = func(name string, num uint8, conf *testutil.TestServerConfig) {
	conf.Performance.RaftMultiplier = 1
	conf.DisableCheckpoint = false
	if num == 0 {
		conf.Bootstrap = true
	} else {
		conf.Bootstrap = false
	}
}

// NewTestCluster will attempt to spin up a cluster of consul test servers of the specified size
func NewTestCluster(name string, size uint8, cb ClusterServerConfigCallback) (*TestCluster, error) {
	var err error

	if size == 0 {
		return nil, errors.New("size must be at least 1")
	}

	cl := &TestCluster{
		name:      name,
		size:      size,
		instances: make([]*TestInstance, size),
	}

	if cb == nil {
		cb = DefaultClusterServerConfigCallback
	}

	cl.instances[0], err = NewTestInstance(fmt.Sprintf("%s-%d", name, 0), func(conf *testutil.TestServerConfig) {
		cb(name, 0, conf)
	})
	if err != nil {
		return nil, err
	}

	if size == 1 {
		return cl, nil
	}

	err = cl.Grow(size-1, cb)
	if err != nil {
		if err != nil {
			ul := len(cl.instances)
			if ul > 0 {
				for u := ul - 1; u >= 0; u-- {
					if !cl.instances[u].Stopped() {
						cl.instances[u].Stop()
					}
				}
			}
			return nil, err
		}
	}

	return cl, nil
}

func (cl *TestCluster) Name() string {
	return cl.name
}

func (cl *TestCluster) Size() int {
	cl.m.Lock()
	defer cl.m.Unlock()
	return len(cl.instances)
}

func (cl *TestCluster) Stopped() bool {
	cl.m.Lock()
	defer cl.m.Unlock()
	return cl.stopped
}

// Stop will attempt to stop the entire cluster.  Once called, the cluster is considered defunct and all further
// interactions will cause a panic
func (cl *TestCluster) Stop() error {
	cl.m.Lock()
	defer cl.m.Unlock()
	if cl.stopped {
		return nil
	}
	return cl.stop()
}

func (cl *TestCluster) stop() error {
	l := len(cl.instances)
	var err error = NewMultiErr(nil)
	for i := 0; i < l; i++ {
		err.(*MultiErr).Add(cl.instances[i].Stop())
	}
	cl.stopped = true
	if err.(*MultiErr).ErrCount() > 0 {
		return err
	}
	return nil
}

func (cl *TestCluster) Instance(num uint8) *TestInstance {
	cl.m.Lock()
	defer cl.m.Unlock()
	if cl.stopped {
		panic(fmt.Sprintf("Cluster %s is defunct", cl.name))
	}
	return cl.instances[num]
}

// Grow will attempt to add n number of test instances to the cluster
func (cl *TestCluster) Grow(n uint8, cb ClusterServerConfigCallback) error {
	cl.m.Lock()
	defer cl.m.Unlock()
	if cl.stopped {
		panic(fmt.Sprintf("Cluster %s is defunct", cl.name))
	}

	current := len(cl.instances)

	if (current + int(n)) > math.MaxUint8 {
		return fmt.Errorf("\"%s\" is already \"%d\" instances long, cannot grow by \"%d\" as it would breach the max allowed cluster instance size of \"%d\"", cl.name, current, n, math.MaxUint8)
	}

	for i := uint8(0); i < n; i++ {
		offset := uint8(current) + i
		instance, err := NewTestInstance(fmt.Sprintf("%s-%d", cl.name, offset), func(conf *testutil.TestServerConfig) {
			cb(cl.name, offset, conf)
		})
		if err != nil {
			return fmt.Errorf("unable to grow \"%s\", instance \"%d\" creation failed: %s", cl.name, offset, err)
		}
		err = cl.instances[0].APIClient().Agent().Join(cl.instances[offset].LANAddr(), false)
		if err != nil {
			return fmt.Errorf("unable to grow \"%s\", instance \"%d\" failed to join: %s", cl.name, offset, err)
		}
		cl.instances = append(cl.instances, instance)
	}

	return nil
}

// Shrink will reduce the # of servers in the cluster, starting with the most recently added.
func (cl *TestCluster) Shrink(n uint8) error {
	cl.m.Lock()
	defer cl.m.Unlock()

	l := len(cl.instances)
	if n > uint8(l) {
		return cl.stop()
	}

	var err error = NewMultiErr(nil)

	diff := uint8(l) - n
	for i := uint8(l); i > diff; i-- {
		err.(*MultiErr).Add(cl.instances[i].Stop())
	}

	cl.instances = cl.instances[0:diff]

	if err.(*MultiErr).ErrCount() > 0 {
		return err
	}
	return nil
}

type AgentMan struct {
	singletons map[string]*TestInstance
	clusters   map[string]*TestCluster
}

func NewAgentMan() *AgentMan {
	am := &AgentMan{
		singletons: make(map[string]*TestInstance),
		clusters:   make(map[string]*TestCluster),
	}

	return am
}