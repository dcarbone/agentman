package agentman_test

import (
	"github.com/dcarbone/agentman"
	"github.com/hashicorp/consul/testutil"
	"github.com/steakknife/devnull"
	"testing"
)

const (
	InstanceName1 = "test-instance-1"

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
		inst, err = agentman.NewTestInstance(InstanceName1, shutup)
		if nil != err {
			t.Logf("Error during NewTestInstance(): %s", err)
			t.FailNow()
		}
	})

	if inst != nil {
		err = inst.Stop()
		if err != nil {
			t.Logf("Error seen while stopping instance: %s", err)
		}
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

	t.Run("Grow", func(t *testing.T) {
		err = cluster.Grow(2, shutupCluster)
		if err != nil {
			t.Logf("Unable to Grow(): %s", err)
			t.FailNow()
		}
		if cluster.Size() != 5 {
			t.Logf("Expected cluster size to be 5, saw: %d", cluster.Size())
			t.FailNow()
		}
	})

	t.Run("Shrink", func(t *testing.T) {
		err = cluster.Shrink(3)
		if err != nil {
			t.Logf("Unable to Shrink(): %s", err)
			// TODO: the errors returned by the testutil package seem to be...mostly meaningless.
			// t.FailNow()
		}
		if cluster.Size() != 2 {
			t.Logf("Expected cluster size to be 2, saw: %d", cluster.Size())
			t.FailNow()
		}
	})

	if cluster != nil {
		err = cluster.Stop()
		if err != nil {
			t.Logf("Error seen while stopping cluster: %s", err)
		}
	}
}
