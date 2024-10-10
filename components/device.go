/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

// Package "components" addresses the structures of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a husk
// (a.k.a. a shell) and a unit-asset (a.k.a. an asset or a thing). The husk runs on a device,
// and exposes the unit assets' functionalities as services.

package components

import (
	"log"
	"net"
	"os"
	"strings"
)

// HostingDevice type holds the attributes of the device on which an Arrowhead framework system is running on
type HostingDevice struct {
	ID           int                 `json:"id"`
	Name         string              `json:"hostname"`
	Certificate  string              `json:"certificate"`
	IPAddresses  []string            `json:"ipAddresses"`
	MACAddresses []string            `json:"macAddresses"`
	Details      map[string][]string `json:"deviceDetails"`
}

// NewDevice constructor gets the device or host name as well as the list of available IPv4 addresses the host has and associated MAC addresses.
func NewDevice() *HostingDevice {
	name, err := Hostname()
	if err != nil {
		log.Fatal(err.Error())
	}
	ipList, err := IpAddresses()
	if err != nil {
		log.Fatalln(err.Error())
	}
	macAddresses, err := MacAddresses(ipList)
	if err != nil {
		log.Fatal(err.Error())
	}
	device := HostingDevice{
		ID:           0,
		Name:         name,
		Certificate:  "",
		IPAddresses:  ipList,
		MACAddresses: macAddresses,
	}
	return &device
}

// Hostname returns the name of the hosting device name within the network
func Hostname() (string, error) {
	name, err := os.Hostname()
	if err != nil {
		log.Println(err.Error())
		return "", err
	}
	return name, nil
}

// IpAddresses returns an array list of IP addresses the device can be accessed with
func IpAddresses() ([]string, error) {
	ipAddresses := make([]string, 0)
	ifaces, err := net.Interfaces()
	if err != nil {
		return ipAddresses, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return ipAddresses, err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			ipAddresses = append(ipAddresses, ip.String())
		}
	}
	ipAddresses = append(ipAddresses, "127.0.0.1") // if the device is not connected to the Internet, it might have the whole local cloud on the device and is at the end of the list

	return ipAddresses, nil
}

// MacAddresses returns the list of physical MAC addresses associated with the list of IP addresses of the hosting device
func MacAddresses(ipAddresses []string) ([]string, error) {
	interfacesList := make([]string, 0)
	ifaces, err := net.Interfaces()
	if err != nil {
		return interfacesList, err
	}

	for _, iface := range ifaces {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				for _, ipAdd := range ipAddresses {

					if strings.Contains(addr.String(), ipAdd) && ipAdd != "127.0.0.1" {
						interfacesList = append(interfacesList, iface.HardwareAddr.String()) // only interested in the name with current IP address
					}
				}
			}
		}
	}

	return interfacesList, nil
}
