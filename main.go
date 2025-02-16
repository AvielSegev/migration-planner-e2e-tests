package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

func runCommand(cmdStr string) {
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to execute: %s, Error: %v", cmdStr, err)
	}
}

func detectPackageManager() string {
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	} else if _, err := exec.LookPath("apt"); err == nil {
		return "apt"
	}
	log.Fatalf("Unsupported package manager. Please use a system with either DNF or APT.")
	return ""
}

func installDependencies() {
	fmt.Println("Installing dependencies...")
	pkgManager := detectPackageManager()
	if pkgManager == "dnf" {
		runCommand("sudo dnf install -y qemu-img sshpass libvirt-devel libvirt-daemon libvirt-daemon-config-network libvirt-daemon-kvm")
	} else if pkgManager == "apt" {
		runCommand("sudo apt update && sudo apt install -y qemu-utils sshpass libvirt-dev libvirt-daemon libvirt-daemon-system")
	}
}

func setupKindCluster(clusterName string) {
	fmt.Println("Setting up Kind Kubernetes cluster...")
	runCommand(fmt.Sprintf("kind create cluster --name %s", clusterName))
}

func deployRegistry() {
	fmt.Println("Deploying local registry...")
	runCommand("kubectl create deployment registry --image=docker.io/registry")
	runCommand("kubectl wait --for=condition=Ready pods --all --timeout=240s")
	runCommand("kubectl port-forward --address 0.0.0.0 deploy/registry 5000:5000 &")
}

func deployVCSIM() {
	fmt.Println("Deploying vSphere simulator...")
	runCommand("make deploy-vsphere-simulator")
	runCommand("kubectl wait --for=condition=Ready pods --all --timeout=240s")
	runCommand("kubectl port-forward --address 0.0.0.0 deploy/vcsim 8989:8989 &")
}

func buildAndDeployContainers() {
	fmt.Println("Building and deploying containers...")
	runCommand("make migration-planner-agent-container MIGRATION_PLANNER_AGENT_IMAGE=$MIGRATION_PLANNER_AGENT_IMAGE")
	runCommand("make migration-planner-api-container MIGRATION_PLANNER_API_IMAGE=$MIGRATION_PLANNER_API_IMAGE")
	runCommand("docker push $MIGRATION_PLANNER_AGENT_IMAGE")
	runCommand("kind load docker-image $MIGRATION_PLANNER_API_IMAGE")
	runCommand("docker rmi $MIGRATION_PLANNER_API_IMAGE")
}

func deployMigrationPlanner() {
	fmt.Println("Deploying assisted migration planner...")
	runCommand("make deploy-on-kind MIGRATION_PLANNER_API_IMAGE=$MIGRATION_PLANNER_API_IMAGE MIGRATION_PLANNER_AGENT_IMAGE=$MIGRATION_PLANNER_AGENT_IMAGE MIGRATION_PLANNER_API_IMAGE_PULL_POLICY=$MIGRATION_PLANNER_API_IMAGE_PULL_POLICY INSECURE_REGISTRY=$INSECURE_REGISTRY MIGRATION_PLANNER_NAMESPACE=default PERSISTENT_DISK_DEVICE=/dev/vda")
	runCommand("kubectl wait --for=condition=Ready pods --all --timeout=240s")
	runCommand("kubectl port-forward --address 0.0.0.0 service/migration-planner-agent 7443:7443 &")
	runCommand("kubectl port-forward --address 0.0.0.0 service/migration-planner 3443:3443 &")
}

func runTests() {
	fmt.Println("Running integration tests...")
	runCommand("mkdir /tmp/untarova")
	runCommand("qemu-img convert -f vmdk -O qcow2 data/persistence-disk.vmdk /tmp/untarova/persistence-disk.qcow2")
	runCommand("sudo make integration-test PLANNER_IP=${REGISTRY_IP}")
}

func printMenu() {
	fmt.Println(
		"Let's run the E2E tests...\n",
		"1 - Prepare Environment\n",
		"2 - Run E2E test\n",
		"3 - Exit and clean\n",
		"4 - Exit")
}

func prepareEnvironment(clusterName string) {
	installDependencies()
	setupKindCluster(clusterName)
	deployRegistry()
	deployVCSIM()
	buildAndDeployContainers()
	deployMigrationPlanner()
}

func cleanEnvironment() {
	fmt.Println("Delete cluster...")
	runCommand("kubectl delete all --all")
	runCommand("kind delete cluster")
}

func main() {
	if len(os.Args) != 1 {
		fmt.Println("Usage: ./env_setup")
		os.Exit(1)
	}

	shouldStop := false

	var operation int
	clusterName := "e2e"

	for {
		printMenu()

		fmt.Print("Please enter Desired operation: ")
		_, _ = fmt.Scanln(&operation)

		switch operation {
		case 1:
			prepareEnvironment(clusterName)
		case 2:
			runTests()
		case 3:
			shouldStop = true
			cleanEnvironment()
		case 4:
			fmt.Println("Exiting...")
			shouldStop = true
		default:
			fmt.Println("Unknown command.")
		}

		if shouldStop {
			break
		}
	}
}
