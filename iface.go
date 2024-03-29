/*
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 * Author: Justin Wong <justin.w.xd@gmail.com>
 *
 */

// Handle virtual interfaces

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/songgao/water"
)

var invalidAddr = errors.New("Invalid device ip address")

var tun_peer net.IP

func newTun(name string) (iface *water.Interface, err error) {

	config := water.Config{
		DeviceType: water.TAP,
	}
	config.Name = name

	//iface, err = water.New(name)
	iface, err = water.New(config)
	if err != nil {
		return nil, err
	}
	fmt.Println("interface %v created", iface.Name())

	sargs := fmt.Sprintf("link set dev %s up mtu %d qlen 100", iface.Name(), MTU)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	fmt.Println("ip %s", sargs)
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	return iface, nil
}

func setTunIP(iface *water.Interface, ip net.IP, subnet *net.IPNet) (err error) {
	ip = ip.To4()
	fmt.Println("%v", ip)
	if ip[3]%2 == 0 {
		return invalidAddr
	}

	peer := net.IP(make([]byte, 4))
	copy([]byte(peer), []byte(ip))
	peer[3]++
	tun_peer = peer

	sargs := fmt.Sprintf("addr add dev %s local %s peer %s", iface.Name(), ip, peer)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	fmt.Println("ip %s", sargs)
	err = cmd.Run()
	if err != nil {
		return err
	}

	sargs = fmt.Sprintf("route add %s via %s dev %s", subnet, peer, iface.Name())
	args = strings.Split(sargs, " ")
	cmd = exec.Command("ip", args...)
	fmt.Println("ip %s", sargs)
	err = cmd.Run()
	return err
}

// return net gateway (default route) and nic
func getNetGateway() (gw, dev string, err error) {

	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", "", err
	}

	defer file.Close()
	rd := bufio.NewReader(file)

	s2byte := func(s string) byte {
		b, _ := strconv.ParseUint(s, 16, 8)
		return byte(b)
	}

	for {
		line, isPrefix, err := rd.ReadLine()

		if err != nil {
			fmt.Println(err.Error())
			return "", "", err
		}
		if isPrefix {
			return "", "", errors.New("Line Too Long!")
		}
		buf := bytes.NewBuffer(line)
		scanner := bufio.NewScanner(buf)
		scanner.Split(bufio.ScanWords)
		tokens := make([]string, 0, 8)

		for scanner.Scan() {
			tokens = append(tokens, scanner.Text())
		}

		iface := tokens[0]
		dest := tokens[1]
		gw := tokens[2]
		mask := tokens[7]

		if bytes.Equal([]byte(dest), []byte("00000000")) &&
			bytes.Equal([]byte(mask), []byte("00000000")) {
			a := s2byte(gw[6:8])
			b := s2byte(gw[4:6])
			c := s2byte(gw[2:4])
			d := s2byte(gw[0:2])

			ip := net.IPv4(a, b, c, d)

			return ip.String(), iface, nil
		}

	}
	return "", "", errors.New("No default gateway found")
}

// add route
func addRoute(dest, nextHop, iface string) {

	scmd := fmt.Sprintf("ip -4 r a %s via %s dev %s", dest, nextHop, iface)
	cmd := exec.Command("bash", "-c", scmd)
	fmt.Println(scmd)
	err := cmd.Run()

	if err != nil {
		fmt.Println(err.Error())
	}

}

// delete route
func delRoute(dest string) {
	sargs := fmt.Sprintf("-4 route del %s", dest)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	fmt.Println("ip %s", sargs)
	err := cmd.Run()

	if err != nil {
		fmt.Println(err.Error())
	}
}

// redirect default gateway
func redirectGateway(iface, gw string) error {
	subnets := []string{"0.0.0.0/1", "128.0.0.0/1"}
	fmt.Println("Redirecting Gateway")
	for _, subnet := range subnets {
		sargs := fmt.Sprintf("-4 route add %s via %s dev %s", subnet, gw, iface)
		args := strings.Split(sargs, " ")
		cmd := exec.Command("ip", args...)
		fmt.Println("ip %s", sargs)
		err := cmd.Run()

		if err != nil {
			return err
		}
	}
	return nil
}

// redirect ports to one
func redirectPort(from, to string) error {
	//iptables -t nat -A PREROUTING -p udp -m udp --dport 40000:41000 -j REDIRECT --to-ports 1234
	fmt.Println("Port Redirecting")
	sargs := fmt.Sprintf("-t nat -A PREROUTING -p udp -m udp --dport %s -j REDIRECT --to-ports %s", from, to)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("iptables", args...)
	err := cmd.Run()

	if err != nil {
		return err
	}
	cmd = exec.Command("ip6tables", args...)
	err = cmd.Run()

	if err != nil {
		return err
	}
	return nil
}

// undo redirect ports
func unredirectPort(from, to string) error {
	//iptables -t nat -D PREROUTING -p udp -m udp --dport 40000:41000 -j REDIRECT --to-ports 1234
	fmt.Println("Clear Port Redirecting")
	sargs := fmt.Sprintf("-t nat -D PREROUTING -p udp -m udp --dport %s -j REDIRECT --to-ports %s", from, to)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("iptables", args...)
	err := cmd.Run()

	if err != nil {
		return err
	}
	cmd = exec.Command("ip6tables", args...)
	err = cmd.Run()

	if err != nil {
		return err
	}
	return nil
}

func fixMSS(iface string, is_server bool) error {
	mss := MTU - 40
	fmt.Println("Fix MSS with iptables to ", mss)
	io := "o"
	if is_server {
		io = "i"
	}
	fmt.Println("io", io)
	fmt.Println("iface", iface)
	sargs := fmt.Sprintf("-I FORWARD -%s %s -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss %d", io, iface, mss)
	fmt.Println("sargs", sargs)
	args := strings.Split(sargs, " ")
	fmt.Println("args", args)
	cmd := exec.Command("iptables", args...)
	fmt.Println("iptables", sargs)
	err := cmd.Run()

	if err != nil {
		return err
	}
	return nil
}

func clearMSS(iface string, is_server bool) error {
	mss := MTU - 40
	fmt.Println("Clean MSS fix")
	io := "o"

	if is_server {
		io = "i"
	}
	sargs := fmt.Sprintf("-D FORWARD -%s %s -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss %d", io, iface, mss)

	args := strings.Split(sargs, " ")
	cmd := exec.Command("iptables", args...)
	err := cmd.Run()

	if err != nil {
		return err
	}

	return nil
}
