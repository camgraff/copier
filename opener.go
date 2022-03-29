package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var version string
var commit string
var date string

type OpenerOptions struct {
	Network string `yaml:"network"`
	Address string `yaml:"address"`
    Timeout int64 `yaml:"timeout"` // in milliseconds
	ErrOut io.Writer
}

func NewOpenerCmd(errOut io.Writer) *cobra.Command {
	var configPath string

	o := &OpenerOptions{
		Network: "unix",
		Address: "~/.copier.sock",
        Timeout: 10,
		ErrOut:  errOut,
	}

	cmd := &cobra.Command{
		Use: "opener",
		RunE: func(_ *cobra.Command, args []string) error {
			if err := LoadOpenerOptionsFromConfig(configPath, o); err != nil {
				return err
			}

			if err := o.Validate(); err != nil {
				return err
			}

			return o.Run()
		},
	}

	cmd.Flags().StringVar(&configPath, "config", configPath, "Path to the opener config file (defaults to ~/.config/opener/config.yaml)")

	return cmd
}

func (o *OpenerOptions) Validate() error {
	switch o.Network {
	case "unix":
		address, err := homedir.Expand(o.Address)
		if err != nil {
			return err
		}
		o.Address = address

		syscall.Umask(0077)

		if err := os.RemoveAll(o.Address); err != nil {
			return err
		}
	case "tcp":
	default:
		return errors.New("allowed network are: unix,tcp")
	}

	return nil
}

func (o *OpenerOptions) Run() error {
	fmt.Fprintf(o.ErrOut, "version: %s, commit: %s, date: %s\n", version, commit, date)
	fmt.Fprintf(o.ErrOut, "starting a server at %s\n", o.Address)

	ln, err := net.Listen(o.Network, o.Address)
	if err != nil {
		return err
	}

	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Fprintln(o.ErrOut, err)
				return
			}

			go handleConnection(conn, o.ErrOut, o.Timeout)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	sig := <-c
	fmt.Fprintf(o.ErrOut, "got signal %s\n", sig)

	return nil
}

func handleConnection(conn net.Conn, errOut io.Writer, timeoutMillis int64) {
	defer conn.Close()
    fmt.Println(timeoutMillis)

    conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMillis * 10e6)))

    data, err := ioutil.ReadAll(conn)
    if err != nil {
        if err, ok := err.(net.Error); !ok || !err.Timeout() {
            fmt.Fprintf(errOut, "failed to read from socket: %v\n", err)
            return
        }
    
    }

	fmt.Fprintf(errOut, "received %q\n", data)

    err = clipboard.WriteAll(string(data))
    if err != nil {
        fmt.Fprintf(errOut, "failed to save to clipboard: %v\n", err)
        if _, err := conn.Write([]byte(err.Error())); err != nil {
            fmt.Fprintf(errOut, "failed to send error to client: %v\n", err)
        }
        return
    }
}
