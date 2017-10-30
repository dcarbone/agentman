package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dcarbone/agentman"
	"github.com/hashicorp/consul/testutil"
	"github.com/steakknife/devnull"
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var (
	quietFlag bool
	debugFlag bool

	cmdFlags          = flag.NewFlagSet("command", flag.ContinueOnError)
	cmdFlagName       string
	cmdFlagStop       bool
	cmdFlagInstance   bool
	cmdFlagCluster    bool
	cmdFlagGrow       bool
	cmdFlagShrink     bool
	cmdFlagSize       uint
	cmdFlagDumpConfig bool

	am = agentman.NewAgentMan()

	cmdLock = new(sync.Mutex)
)

func log(d bool, v ...interface{}) {
	if quietFlag {
		return
	}
	if d && !debugFlag {
		return
	}
	stdlog.Print(v...)
}

func logf(d bool, format string, v ...interface{}) {
	if quietFlag {
		return
	}
	if d && !debugFlag {
		return
	}
	stdlog.Printf(format, v...)
}

func instanceCommand() {
	if cmdFlagCluster {
		fmt.Fprint(os.Stdout, "Cannot specify -instance and -cluster at the same time\n")
	} else if cmdFlagGrow {
		fmt.Fprint(os.Stdout, "-grow not usable with -instance\n")
	} else if cmdFlagShrink {
		fmt.Fprint(os.Stdout, "-shrink not usable with -instance\n")
	} else if cmdFlagSize != 0 {
		fmt.Fprint(os.Stdout, "-size not usable with -instance\n")
	} else if cmdFlagStop {
		am.StopInstance(cmdFlagName)
	} else if cmdFlagDumpConfig {
		inst, ok := am.Instance(cmdFlagName)
		if ok {
			// TODO: this is a bad idea....
			b, _ := json.Marshal(inst.Config())
			fmt.Fprintf(os.Stdout, "%s\n", string(b))
		} else {
			fmt.Fprint(os.Stdout, "{}\n")
		}
	} else {
		inst, err := am.NewInstance(cmdFlagName, func(conf *testutil.TestServerConfig) {
			conf.Stdout = devnull.Writer
			conf.Stderr = devnull.Writer
		})
		if err != nil {
			fmt.Fprintf(os.Stdout, "Unable to start instance: %s\n", err)
			return
		}
		b, _ := json.Marshal(inst.Config())
		fmt.Fprintf(os.Stdout, "%s\n", string(b))
	}
}

func clusterCommand() {
	if cmdFlagInstance {
		fmt.Fprint(os.Stdout, "Cannot specify -instance and -cluster at the same time\n")
	} else if cmdFlagGrow {
		if cmdFlagShrink {
			fmt.Fprint(os.Stdout, "Cannot specify -shrink and -grow at the same time\n")
		}
	} else if cmdFlagDumpConfig {
		configs := make([]*testutil.TestServerConfig, 0)
		cluster, ok := am.Cluster(cmdFlagName)
		if ok {
			// TODO: this is a bad idea....
			for i := 0; i < cluster.Size(); i++ {
				inst := cluster.Instance(uint8(i))
				configs = append(configs, inst.Config())
			}
			b, _ := json.Marshal(configs)
			fmt.Fprintf(os.Stdout, "%s\n", string(b))
		} else {
			fmt.Fprint(os.Stdout, "[]\n")
		}
	}
}

func parseNewCmd(input string) {
	cmdLock.Lock()
	defer cmdLock.Unlock()

	err := cmdFlags.Parse(strings.Split(input, " "))
	if err != nil {
		fmt.Fprintf(os.Stdout, "Unable to parse input: %s\n", err)
		return
	}

	if cmdFlagName == "" {
		fmt.Fprint(os.Stdout, "-name must be populated\n")
		return
	}

	if !cmdFlagInstance && !cmdFlagCluster {
		fmt.Fprint(os.Stdout, "-instance or -cluster must be specified")
		return
	}

	if cmdFlagInstance {
		instanceCommand()
	} else {
		clusterCommand()
	}
}

func main() {
	flag.BoolVar(&quietFlag, "quiet", false, "Enable quiet mode")
	flag.BoolVar(&debugFlag, "debug", false, "Enable debug mode")
	flag.Parse()

	log(false, "Booting up AgentMan daemon...")

	cmdLock = new(sync.Mutex)

	am = agentman.NewAgentMan()

	cmdFlags = flag.NewFlagSet("command", flag.ContinueOnError)
	cmdFlags.StringVar(&cmdFlagName, "name", "", "Name of instance or cluster to perform action on")
	cmdFlags.BoolVar(&cmdFlagStop, "stop", false, "Stop instance or cluster -name")
	cmdFlags.BoolVar(&cmdFlagInstance, "instance", false, "Create or Stop instance -name")
	cmdFlags.BoolVar(&cmdFlagCluster, "cluster", false, "Create or Stop cluster -name")
	cmdFlags.BoolVar(&cmdFlagGrow, "grow", false, "Grow cluster -name by -size")
	cmdFlags.BoolVar(&cmdFlagShrink, "shrink", false, "Shrink cluster -name by -size")
	cmdFlags.UintVar(&cmdFlagSize, "size", 0, "Amount to create, grow, or shrink cluster -name by")
	cmdFlags.BoolVar(&cmdFlagDumpConfig, "dump-config", false, "Dump configuration of instance or cluster -name")

	done := make(chan struct{})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGINFO)

	stdinChan := make(chan string, 10)
	reader := bufio.NewReader(os.Stdin)

	go func() {
		defer close(done)
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				logf(false, "Unable to read from stdin: %s", err)
				return
			}
			stdinChan <- strings.TrimSpace(input)
		}
	}()

	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				logf(false, "Saw signal %s, shutting down...", sig)
				err := am.Stop()
				if err != nil {
					logf(false, "Did not shut down cleanly: %s", err)
					os.Exit(1)
				} else {
					os.Exit(0)
				}
			case syscall.SIGINFO:
				fmt.Fprintf(os.Stdout, "Instances: [\"%s\"]; Clusters: [\"%s\"];", strings.Join(am.InstanceNames(), "\", \""), strings.Join(am.ClusterNames(), "\", \""))
			}
		case <-done:
			os.Exit(0)
		case cmd := <-stdinChan:
			parseNewCmd(cmd)
		}
	}
}
