// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package svchelper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var elog debug.Log

type Service interface {
	Schedule(ctx context.Context, wg *sync.WaitGroup, cancel context.CancelFunc) error
}

type ServiceWrapper struct {
	service                      Service
	serviceName                  string
	serviceDisplayName           string
	serviceDescription           string
	useExePathAsWorkingDirectory bool
}

func GetServiceWrapper(service Service, servicName, serviceDisplayName, serviceDescription string, useExePathAsWorkingDirectory bool) (*ServiceWrapper, error) {
	if useExePathAsWorkingDirectory {
		if err := setExePathAsWorkingDirectory(); err != nil {
			return nil, fmt.Errorf("when changing working directory: %s", err)
		}
	}
	return &ServiceWrapper{
		service:                      service,
		serviceName:                  servicName,
		serviceDisplayName:           serviceDisplayName,
		serviceDescription:           serviceDescription,
		useExePathAsWorkingDirectory: useExePathAsWorkingDirectory,
	}, nil
}

func setExePathAsWorkingDirectory() error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("when getting executable path: %s", err)
	}
	executableDir := filepath.Dir(executablePath)
	if err := os.Chdir(executableDir); err != nil {
		return fmt.Errorf("when changing to executable path: %s", err)
	}
	return nil
}

func (sw *ServiceWrapper) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown // | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	if err := sw.service.Schedule(ctx, wg, cancel); err != nil {
		elog.Error(1, fmt.Sprintf("When scheduling the service '%s': %s", sw.serviceName, err))
		cancel()
		wg.Wait()
		errno = 1
		return
	}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
loop:
	for {
		select {
		case <-ctx.Done():
			elog.Info(1, "The wrapped service cancelled the execution")
			wg.Wait()
			errno = 0
			break loop
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				// golang.org/x/sys/windows/svc.TestExample is verifying this output.
				testOutput := strings.Join(args, "-")
				testOutput += fmt.Sprintf("-%d", c.Context)
				elog.Info(1, testOutput)
				cancel()
				wg.Wait()
				break loop
			default:
				elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}

func (sw *ServiceWrapper) RunService(isDebug bool) error {
	var err error
	if isDebug {
		elog = debug.New(sw.serviceName)
	} else {
		elog, err = eventlog.Open(sw.serviceName)
		if err != nil {
			return fmt.Errorf("when opening the eventlog: %w", err)
		}
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("starting %s service", sw.serviceName))
	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	if err = run(sw.serviceName, sw); err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", sw.serviceName, err))
		return err
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", sw.serviceName))
	return nil
}
