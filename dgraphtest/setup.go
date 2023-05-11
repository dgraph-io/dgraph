package dgraphtest

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type ResourceDetails interface {
	User() string
	Keys() string
	IP() string
}

type DevMachine struct {
}

func (d DevMachine) User() string {
	return "dev"
}

func (d DevMachine) Keys() string {
	return ""
}

func (d DevMachine) IP() string {
	return ""
}

type AWSDetails struct {
	user string
	keys string
	ip   string
}

func (a AWSDetails) User() string {
	return a.user
}

func (a AWSDetails) Keys() string {
	return a.keys
}

func (a AWSDetails) IP() string {
	return a.ip
}

// TODO (anurag): This can sit in a different job
// If cluster has been setup already than read the details from the config file
// and return the details
func ProvisionClientAndTarget(task DgraphPerf) ResourceDetails {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("Error loading .env file")
	}
	switch task.rc.loc {
	case "aws":
		return AWSDetails{
			user: os.Getenv("USER"),
			keys: os.Getenv("KEYS"),
			ip:   os.Getenv("IP"),
		}
	default:
		return DevMachine{}
	}
}
