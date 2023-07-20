// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package svchelper

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func (sw *ServiceWrapper) usage(errmsg string) {
	fmt.Fprintf(os.Stderr,
		"%s\n\n"+
			"usage: %s <command>\n"+
			"       where <command> is one of\n"+
			"       install, remove, debug, start, stop, pause or continue.\n",
		errmsg, os.Args[0])
	os.Exit(2)
}

func (sw *ServiceWrapper) ManageService() error {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to determine if we are running in service: %w", err)
	}
	if inService {
		return sw.RunService(false)
	}

	if len(os.Args) < 2 {
		sw.usage("no command specified")
	}

	cmd := strings.ToLower(os.Args[1])
	switch cmd {
	case "debug":
		err = sw.RunService(true)
	case "install":
		err = sw.InstallService()
	case "remove":
		err = sw.RemoveService()
	case "start":
		err = sw.StartService()
	case "stop":
		err = sw.ControlService(svc.Stop, svc.Stopped)
	case "pause":
		err = sw.ControlService(svc.Pause, svc.Paused)
	case "continue":
		err = sw.ControlService(svc.Continue, svc.Running)
	default:
		sw.usage(fmt.Sprintf("invalid command %s", cmd))
	}
	if err != nil {
		return fmt.Errorf("failed to %s %s: %v", cmd, sw.serviceName, err)
	}
	return nil
}

func (sw *ServiceWrapper) StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(sw.serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func (sw *ServiceWrapper) ControlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(sw.serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}
