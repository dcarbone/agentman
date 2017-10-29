package agentman

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testutil"
	"log"
	"math"
	"net/http"
	"sync"
)

// TestInstance represents a single instance of a consul test server and its client.  May be alone or in a cluster.
type TestInstance struct {
	m *sync.Mutex

	name string

	server *testutil.TestServer
	client *api.Client
}

// NewTestInstance will attempt to create a new consul test server and api client
func NewTestInstance(name string, cb testutil.ServerConfigCallback) (*TestInstance, error) {
	var err error
	s := &TestInstance{
		m:    new(sync.Mutex),
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
		m *sync.Mutex

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
		m:         new(sync.Mutex),
		name:      name,
		size:      size,
		instances: make([]*TestInstance, 1, math.MaxUint8),
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
					cl.instances[u].Stop()
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
	if l == 0 {
		return nil
	}

	var err error = NewMultiErr()
	for i := l - 1; i >= 0; i-- {
		err.(*MultiErr).Add(cl.instances[i].Stop())
	}

	cl.stopped = true

	if err.(*MultiErr).Size() > 0 {
		return err
	}
	return nil
}

// Instance will attempt to return a single instance from this cluster
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
		err = cl.instances[0].APIClient().Agent().Join(instance.LANAddr(), false)
		if err != nil {
			instance.Stop()
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

	var err error = NewMultiErr()

	diff := uint8(l) - n
	for i := uint8(l - 1); i > diff; i-- {
		err.(*MultiErr).Add(cl.instances[i].Stop())
	}

	cl.instances = cl.instances[0:diff]

	if err.(*MultiErr).Size() > 0 {
		return err
	}
	return nil
}

type (
	Singles  map[string]*TestInstance
	Clusters map[string]*TestCluster

	AgentMan struct {
		m        sync.Mutex
		singles  Singles
		clusters Clusters
	}
)

func NewAgentMan() *AgentMan {
	am := &AgentMan{
		singles:  make(Singles),
		clusters: make(Clusters),
	}

	return am
}

// NewSingle will attempt to create an un-clustered test instance
func (am *AgentMan) NewSingle(name string, cb testutil.ServerConfigCallback) (*TestInstance, error) {
	am.m.Lock()
	defer am.m.Unlock()
	if _, ok := am.singles[name]; ok {
		return nil, fmt.Errorf("single \"%s\" already exists", name)
	}

	s, err := NewTestInstance(name, cb)
	if err != nil {
		return nil, err
	}

	am.singles[name] = s
	return s, nil
}

// NewCluster will attempt to create a clustered set of test instances
func (am *AgentMan) NewCluster(name string, size uint8, cb ClusterServerConfigCallback) (*TestCluster, error) {
	am.m.Lock()
	defer am.m.Unlock()
	if _, ok := am.clusters[name]; ok {
		return nil, fmt.Errorf("cluster \"%s\" already exists", name)
	}

	cl, err := NewTestCluster(name, size, cb)
	if err != nil {
		return nil, err
	}

	am.clusters[name] = cl
	return cl, nil
}

// Single will attempt to return a registered non-clustered test instance to you
func (am *AgentMan) Single(name string) (*TestInstance, bool) {
	am.m.Lock()
	defer am.m.Unlock()
	s, ok := am.singles[name]
	return s, ok
}

// Cluster will attempt to return a registered test cluster to you
func (am *AgentMan) Cluster(name string) (*TestCluster, bool) {
	am.m.Lock()
	defer am.m.Unlock()
	cl, ok := am.clusters[name]
	return cl, ok
}

// StopSingle will attempt to stop a single instance, removing it from this manager
func (am *AgentMan) StopSingle(name string) error {
	am.m.Lock()
	defer am.m.Unlock()

	var err error

	if s, ok := am.singles[name]; ok {
		err = s.Stop()
		delete(am.singles, name)
	}

	return err
}

// StopCluster will attempt to stop a single cluster, removing it from this manager
func (am *AgentMan) StopCluster(name string) error {
	am.m.Lock()
	defer am.m.Unlock()

	var err error

	if cl, ok := am.clusters[name]; ok {
		err = cl.Stop()
		delete(am.clusters, name)
	}

	return err
}

// Stop will attempt to stop all currently running singles and clusters, removing all of them from the manager
func (am *AgentMan) Stop() error {
	am.m.Lock()
	defer am.m.Unlock()

	var errs error = NewMultiErr()

	wg := new(sync.WaitGroup)

	// TODO: maybe per single and per cluster?
	wg.Add(2)

	go func() {
		for _, instance := range am.singles {
			errs.(*MultiErr).Add(instance.Stop())
		}
		wg.Done()
	}()

	go func() {
		for _, cluster := range am.clusters {
			errs.(*MultiErr).Add(cluster.Stop())
		}
		wg.Done()
	}()

	wg.Wait()

	am.singles = make(Singles)
	am.clusters = make(Clusters)

	if errs.(*MultiErr).Size() > 0 {
		return errs
	}

	return nil
}
