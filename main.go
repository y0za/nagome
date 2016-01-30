package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// Application holds app settings and valuables
type Application struct {
	// SavePath is directory to hold save files
	SavePath string
	// Version is version info
	Version string
	// Name is name of this app
	Name string
}

var (
	// App is global Application settings and valuables for this app
	App Application
	// Logger is logger in this app
	Logger       *log.Logger
	printVersion bool
	printHelp    bool
)

func main() {
	flag.Parse()

	if printHelp {
		flag.Usage()
		return
	}
	if printVersion {
		fmt.Println(App.Name, " ", App.Version)
		return
	}

	err := os.MkdirAll(App.SavePath, 0777)
	if err != nil {
		log.Fatal("could not make save directory\n" + err.Error())
	}

	file, err := os.Create(filepath.Join(App.SavePath, "info.log"))
	if err != nil {
		log.Fatal("could not open log file\n" + err.Error())
	}
	defer file.Close()
	Logger = log.New(file, "", log.Lshortfile|log.Ltime)

	Logger.Println("kepe")

	fmt.Println("Hello ", App.Name)

	return
}

func init() {
	App.Version = "0.0"
	App.Name = "Nagome"

	// set command line options
	flag.StringVar(&App.SavePath, "savepath",
		findUserConfigPath(), "Set <directory> to save directory.")
	flag.BoolVar(&printHelp, "h", false, "Print this help.")
	flag.BoolVar(&printHelp, "help", false, "Print this help.")
	flag.BoolVar(&printVersion, "version", false, "Print version information.")

	return
}

func findUserConfigPath() string {
	var home, dir string

	switch runtime.GOOS {
	case "windows":
		home = os.Getenv("USERPROFILE")
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(home, "Application Data")
		}
	case "plan9":
		home = os.Getenv("home")
		dir = filepath.Join(home, ".config")
	default:
		home = os.Getenv("HOME")
		dir = filepath.Join(home, ".config")
	}

	return filepath.Join(dir, App.Name)
}
