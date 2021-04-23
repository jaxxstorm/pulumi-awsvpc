package provider

import (
	"math"
	"net"

	"github.com/apparentlymart/go-cidr/cidr"
)

func SubnetDistributor(baseCidr string, azCount int) ([]string, []string, error) {
	bits := math.Log2(float64(nextPowerof2(azCount)))

	var privateSubnet []string
	var publicSubnet []string

	for i := 0; i < azCount; i++ {
		baseCidr, err := cidrSubnet(baseCidr, int(bits), i)
		if err != nil {
			return nil, nil, err
		}

		private, _ := cidrSubnet(baseCidr, 1, 0)
		splitBase, _ := cidrSubnet(baseCidr, 1, 1)
		public, _ := cidrSubnet(splitBase, 1, 0)

		privateSubnet = append(privateSubnet, private)
		publicSubnet = append(publicSubnet, public)
	}

	return privateSubnet, publicSubnet, nil

}

// calculate the next power of 2
func nextPowerof2(v int) int {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func cidrSubnet(baseAddr string, prefix int, subnetNumber int) (string, error) {
	_, ipnet, err := net.ParseCIDR(baseAddr)
	if err != nil {
		return "", err
	}
	newSubnet, err := cidr.Subnet(ipnet, prefix, subnetNumber)
	if err != nil {
		return "", err
	}
	return newSubnet.String(), nil

}
