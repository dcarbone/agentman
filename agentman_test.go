package agentman_test

import (
	"github.com/dcarbone/agentman"
	"github.com/hashicorp/consul/testutil"
	"github.com/steakknife/devnull"
	"testing"
)

const (
	InstName1 = "test-inst-1"

	ClusterName1 = "test-cluster-1"
)

func shutup(conf *testutil.TestServerConfig) {
	conf.Stdout = devnull.Writer
	conf.Stderr = devnull.Writer
}

func shutupCluster(_ string, _ uint8, conf *testutil.TestServerConfig) {
	conf.Stdout = devnull.Writer
	conf.Stderr = devnull.Writer
}

func TestTestInstance(t *testing.T) {
	var inst *agentman.TestInstance
	var err error

	t.Run("New", func(t *testing.T) {
		inst, err = agentman.NewTestInstance(InstName1, shutup)
		if nil != err {
			t.Logf("Error during NewTestInstance(): %s", err)
			t.FailNow()
		}
	})

	if inst != nil {
		inst.Stop()
	}
}

func TestTestCluster(t *testing.T) {
	var cluster *agentman.TestCluster
	var err error

	t.Run("New", func(t *testing.T) {
		cluster, err = agentman.NewTestCluster(ClusterName1, 3, shutupCluster)
		if nil != err {
			t.Logf("Error during NewTestCluster(): %s", err)
			t.FailNow()
		}
	})

	if cluster != nil {
		cluster.Stop()
	}
}
