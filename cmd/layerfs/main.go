package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	layerfs "github.com/anouaressa/layerfs-simulator"
)

func main() {
	fs := layerfs.New(
		layerfs.Layer{
			"etc/app.conf":    []byte("theme=light\n"),
			"docs/readme.txt": []byte("This file comes from the base layer.\n"),
		},
		layerfs.Layer{
			"etc/app.conf":     []byte("theme=default\n"),
			"var/log/demo.log": []byte("simulator ready\n"),
		},
	)

	fmt.Println("LayerFS simulator — type 'help' for commands, 'exit' to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("layerfs> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		command, argument, _ := strings.Cut(line, " ")
		argument = strings.TrimSpace(argument)
		switch command {
		case "exit", "quit":
			return
		case "help":
			fmt.Println("ls [path] | cat <path> | write <path> <text> | rm <path> | layers | exit")
		case "ls":
			entries, err := fs.ReadDir(argument)
			if err != nil {
				fmt.Println("error:", err)
				continue
			}
			for _, entry := range entries {
				if entry.IsDir {
					fmt.Printf("<dir>  %s/\n", entry.Name)
				} else {
					fmt.Printf("%5d  %s\n", entry.Size, entry.Name)
				}
			}
		case "cat":
			data, err := fs.ReadFile(argument)
			if err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Print(string(data))
			}
		case "write":
			file, text, found := strings.Cut(argument, " ")
			if !found {
				fmt.Println("usage: write <path> <text>")
				continue
			}
			if err := fs.WriteFile(file, []byte(text+"\n")); err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Println("written to upper layer:", file)
			}
		case "rm":
			if err := fs.Remove(argument); err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Println("hidden from merged view:", argument)
			}
		case "layers":
			fmt.Println("visible files:", fs.VisibleFiles())
			fmt.Println("upper files:  ", fs.UpperFiles())
			fmt.Println("whiteouts:    ", fs.Whiteouts())
		default:
			fmt.Println("unknown command; type 'help'")
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "input error:", err)
		os.Exit(1)
	}
}
