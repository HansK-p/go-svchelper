// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package svchelper

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func (sw *ServiceWrapper) ExePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, err = os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		}
	}
	return "", err
}

func (sw *ServiceWrapper) InstallService() error {
	exepath, err := sw.ExePath()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(sw.serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", sw.serviceName)
	}
	s, err = m.CreateService(sw.serviceName, exepath, mgr.Config{DisplayName: sw.serviceDisplayName, Description: sw.serviceDescription, StartType: mgr.StartAutomatic}, "is", "auto-started")
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(sw.serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}
	return nil
}

func (sw *ServiceWrapper) RemoveService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(sw.serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", sw.serviceName)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(sw.serviceName)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return nil
}
