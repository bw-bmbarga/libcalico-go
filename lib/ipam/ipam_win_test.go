// Copyright (c) 2018-2019 Tigera, Inc. All rights reserved.

package ipam

import (
	"context"
	"fmt"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/bw-bmbarga/libcalico-go/lib/apiconfig"
	"github.com/bw-bmbarga/libcalico-go/lib/backend"
	bapi "github.com/bw-bmbarga/libcalico-go/lib/backend/api"
	"github.com/bw-bmbarga/libcalico-go/lib/backend/k8s"
	"github.com/bw-bmbarga/libcalico-go/lib/backend/model"
	cnet "github.com/bw-bmbarga/libcalico-go/lib/net"
	"github.com/bw-bmbarga/libcalico-go/lib/testutils"
)

// Simulating ipamClient Interface
type ipamClientWindows struct {
	client            bapi.Client
	pools             ipPoolAccessor
	blockReaderWriter blockReaderWriter
}

//Returns the block CIDR for the given IP
func (c ipamClientWindows) GetAssignmentBlockCIDR(ctx context.Context, addr cnet.IP) cnet.IPNet {
	pool, err := c.blockReaderWriter.getPoolForIP(addr, nil)
	Expect(err).NotTo(HaveOccurred())
	blockCIDR := getBlockCIDRForAddress(addr, pool)
	return blockCIDR
}

var (
	ipPoolsWindows  = &ipPoolAccessor{pools: map[string]pool{}}
	rsvdAttrWindows = &HostReservedAttr{
		StartOfBlock: 3,
		EndOfBlock:   1,
		Handle:       WindowsReservedHandle,
		Note:         "ipam ut",
	}

	// With default block size 26 or 122, there are 64 ips in one block.
	rsvdAttrTooBig = &HostReservedAttr{
		StartOfBlock: 32,
		EndOfBlock:   33,
		Handle:       WindowsReservedHandle,
		Note:         "ipam ut",
	}
)

type testArgsClaimAff1 struct {
	inNet, host                 string
	cleanEnv                    bool
	pool                        []string
	assignIP                    net.IP
	expClaimedIPs, expFailedIPs int
	expError                    error
}

var _ = testutils.E2eDatastoreDescribe("Windows: IPAM tests", testutils.DatastoreEtcdV3, func(config apiconfig.CalicoAPIConfig) {
	var bc bapi.Client
	var ic Interface
	var kc *kubernetes.Clientset

	BeforeEach(func() {
		// Create a new backend client and an IPAM Client using the IP Pools Accessor.
		// Tests that need to ensure a clean datastore should invokke Clean() on the datastore at the start of the
		// tests.
		var err error
		bc, err = backend.NewClient(config)
		Expect(err).NotTo(HaveOccurred())
		ic = NewIPAMClient(bc, ipPoolsWindows)

		// If running in KDD mode, extract the k8s clientset.
		if config.Spec.DatastoreType == "kubernetes" {
			kc = bc.(*k8s.KubeClient).ClientSet
		}
	})

	It("Windows: Should return error if strict affinity is false for Windows", func() {
		bc.Clean()
		deleteAllPoolsWindows()
		// Hosts must exist before trying to autoassign
		err := applyNode(bc, kc, "Windows-TestHost-1", nil)
		Expect(err).NotTo(HaveOccurred())
		defer deleteNode(bc, kc, "Windows-TestHost-1")

		ipPoolsWindows.pools["100.0.0.0/24"] = pool{cidr: "100.0.0.0/24", enabled: true, blockSize: 26}

		fromPool := cnet.MustParseNetwork("100.0.0.0/24")

		// Windows Hosts
		By("Trying to allocate an ip for windows host 1")
		ctx1 := context.WithValue(context.Background(), "windowsHost", "windows")
		args1 := AutoAssignArgs{
			Num4:                  1,
			Num6:                  0,
			Hostname:              "Windows-TestHost-1",
			IPv4Pools:             []cnet.IPNet{fromPool},
			HostReservedAttrIPv4s: rsvdAttrWindows,
		}
		_, _, outErr := ic.AutoAssign(ctx1, args1)
		Expect(outErr).To(Equal(ErrStrictAffinity))
	})

	// Request for 256 IPs from a pool, say "10.0.0.0/24", with a blocksize of 26, allocates only 240 IPs as
	// the pool of 256 IPs is splitted into 4 blocks of 64 IPs each and 4 IPs, i.e,
	// gateway IP, the first IP of the block, the second IP of the block and the broadcast IP are reserved.
	// So 256 - (4 * 4) = 240.

	// Test Case: Reserved IPs should not be allocated
	//            test case is only written for IPv4
	DescribeTable("Windows: IPAM AutoAssign should not assign reserved IPs",
		func(host string, cleanEnv bool, pools []pool, usePool string, inv4 int, expv4 int, expError error, windowsHost string) {
			if cleanEnv {
				bc.Clean()
				deleteAllPoolsWindows()
			}

			setAffinity(ic, true)
			defer setAffinity(ic, false)

			for _, v := range pools {
				ipPoolsWindows.pools[v.cidr] = pool{cidr: v.cidr, enabled: v.enabled, blockSize: v.blockSize}
			}

			// Host must exist before trying to autoassign to it
			err := applyNode(bc, kc, host, nil)
			Expect(err).NotTo(HaveOccurred())
			defer deleteNode(bc, kc, host)

			fromPool := cnet.MustParseNetwork(usePool)
			args := AutoAssignArgs{
				Num4:                  inv4,
				Num6:                  0,
				Hostname:              host,
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}

			ctx := context.WithValue(context.Background(), "windowsHost", windowsHost)
			outv4, _, outErr := ic.AutoAssign(ctx, args)
			if expError != nil {
				Expect(outErr).To(HaveOccurred())
			} else {
				Expect(outErr).ToNot(HaveOccurred())
			}

			Expect(len(outv4)).To(Equal(expv4))

			reservedIPs := []string{
				"100.0.0.0", "100.0.0.1", "100.0.0.2", "100.0.0.63",
				"100.0.0.64", "100.0.0.65", "100.0.0.66", "100.0.0.127",
				"100.0.0.128", "100.0.0.129", "100.0.0.130", "100.0.0.191",
				"100.0.0.192", "100.0.0.193", "100.0.0.194", "100.0.0.255",
			}

			for _, ip := range outv4 {
				Expect(reservedIPs).NotTo(ContainElement(ip.String()))
			}

		},

		// Test 1: AutoAssign 256 IPv4 - expect NOT to assign 100.0.0.0, 100.0.0.1, 100.0.0.2, 100.0.0.63,
		//	   					      100.0.0.64, 100.0.0.65, 100.0.0.66, 100.0.0.127,
		//                                                    100.0.0.128, 100.0.0.129, 100.0.0.130, 100.0.0.191,
		//                                                    100.0.0.192, 100.0.0.193, 100.0.0.194, 100.0.0.255 IPs.
		Entry("256 v4 ", "testHost", true, []pool{{cidr: "100.0.0.0/24", blockSize: 26, enabled: true}}, "100.0.0.0/24", 256, 240, nil, "windows"),
	)

	// This test is to check if Windows host runs out of IPs from the block with which it has affinity, then IPs from other blocks should not be assigned.
	// Below test creates 2 windows hosts and 2 linux hosts. Initially each of the hosts are assigned 1 IP each from different blocks.
	// The pool of IPs considered for this case provides exactly 4 blocks of IPs.
	// Request for another 100 IPs by any Windows host, created initially, will get only 59 IPs.
	// Request for another 100 IPs by a Linux host, created initially, will get all 100 IPs.
	// Request for another 100 IPs by the other Linux host, created initially, will not get all 100 IPs as all the IPs exhausted.
	Describe("Windows: IPAM AutoAssign should not assign IPs from non-affine block for Windows", func() {

		BeforeEach(func() {
			bc.Clean()
			deleteAllPoolsWindows()
			// Hosts must exist before trying to autoassign
			err := applyNode(bc, kc, "Windows-TestHost-1", nil)
			Expect(err).NotTo(HaveOccurred())
			err = applyNode(bc, kc, "Windows-TestHost-2", nil)
			Expect(err).NotTo(HaveOccurred())
			err = applyNode(bc, kc, "Linux-TestHost-1", nil)
			Expect(err).NotTo(HaveOccurred())
			err = applyNode(bc, kc, "Linux-TestHost-2", nil)
			Expect(err).NotTo(HaveOccurred())

			setAffinity(ic, true)
		})

		AfterEach(func() {
			setAffinity(ic, false)
			deleteNode(bc, kc, "Windows-TestHost-1")
			deleteNode(bc, kc, "Windows-TestHost-2")
			deleteNode(bc, kc, "Linux-TestHost-1")
			deleteNode(bc, kc, "Linux-TestHost-2")
		})

		It("Windows: Should not be able to assign IPs from non-affine block for Windows and Linux", func() {
			ipPoolsWindows.pools["100.0.0.0/24"] = pool{cidr: "100.0.0.0/24", enabled: true, blockSize: 26}

			fromPool := cnet.MustParseNetwork("100.0.0.0/24")

			// Windows Hosts
			By("Trying to allocate an ip for windows host 1")
			ctx1 := context.WithValue(context.Background(), "windowsHost", "windows")
			args1 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              "Windows-TestHost-1",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_1, _, outErr := ic.AutoAssign(ctx1, args1)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_1)).To(Equal(1))

			By("Trying to allocate an ip for windows host 2")
			args2 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              "Windows-TestHost-2",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_2, _, outErr := ic.AutoAssign(ctx1, args2)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_2)).To(Equal(1))

			// Linux Hosts
			By("Trying to allocate an ip for linux host 1")
			ctx2 := context.WithValue(context.Background(), "windowsHost", "linux")
			args3 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              "Linux-TestHost-1",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_3, _, outErr := ic.AutoAssign(ctx2, args3)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_3)).To(Equal(1))

			By("Trying to allocate an ip for linux host 2")
			args4 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              "Linux-TestHost-2",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}

			outv4_4, _, outErr := ic.AutoAssign(ctx2, args4)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_4)).To(Equal(1))

			By("Trying to allocate 100 IPs for windows host 1")
			args5 := AutoAssignArgs{
				Num4:                  100,
				Num6:                  0,
				Hostname:              "Windows-TestHost-1",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_5, _, outErr := ic.AutoAssign(ctx1, args5)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_5)).To(Equal(59))

			By("Trying to allocate 100 IPs for linux host 1")
			args6 := AutoAssignArgs{
				Num4:                  100,
				Num6:                  0,
				Hostname:              "Linux-TestHost-1",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_6, _, outErr := ic.AutoAssign(ctx2, args6)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_6)).To(Equal(59))

			By("Trying to allocate 100 IPs for linux host 2")
			args7 := AutoAssignArgs{
				Num4:                  100,
				Num6:                  0,
				Hostname:              "Linux-TestHost-2",
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}
			outv4_7, _, outErr := ic.AutoAssign(ctx2, args7)
			Expect(outErr).ToNot(HaveOccurred())
			Expect(len(outv4_7)).To(Equal(59))
		})
	})

	Describe("Windows: IPAM AutoAssign from any pool", func() {
		// Assign an IP address, don't pass a pool, make sure we can get an
		// address.
		args := AutoAssignArgs{
			Num4:                  1,
			Num6:                  0,
			Hostname:              "test-host",
			HostReservedAttrIPv4s: rsvdAttrWindows,
		}

		BeforeEach(func() {
			// Hosts must exist before trying to autoassign
			err := applyNode(bc, kc, "test-host", nil)
			Expect(err).NotTo(HaveOccurred())

			setAffinity(ic, true)
		})

		AfterEach(func() {
			deleteNode(bc, kc, "test-host")
			setAffinity(ic, false)
		})

		// Call once in order to assign an IP address and create a block.
		It("Windows: should have assigned an IP address with no error", func() {
			deleteAllPoolsWindows()
			ipPoolsWindows.pools["100.0.0.0/24"] = pool{cidr: "100.0.0.0/24", enabled: true, blockSize: 26}
			ipPoolsWindows.pools["200.0.0.0/24"] = pool{cidr: "200.0.0.0/24", enabled: true, blockSize: 26}
			ctx := context.WithValue(context.Background(), "windowsHost", "windows")
			v4, _, outErr := ic.AutoAssign(ctx, args)
			Expect(outErr).NotTo(HaveOccurred())
			Expect(len(v4) == 1).To(BeTrue())
			Expect(checkWindowsValidIP(v4[0].IP, 26)).To(BeTrue())
			Expect(isValidWindowsHandle(bc, ipPoolsWindows, v4[0].IP, ctx)).To(BeTrue())

			By("Calling again to trigger an assignment from the newly created block.")
			v4_next, _, outErr := ic.AutoAssign(ctx, args)
			Expect(outErr).NotTo(HaveOccurred())
			Expect(len(v4_next)).To(Equal(1))
			Expect(checkWindowsValidIP(v4_next[0].IP, 26)).To(BeTrue())
			Expect(isValidWindowsHandle(bc, ipPoolsWindows, v4_next[0].IP, ctx)).To(BeTrue())
		})

	})

	Describe("Windows: IPAM AutoAssign from different pools", func() {
		host := "host-A"
		pool1 := cnet.MustParseNetwork("100.0.0.0/24")
		pool2 := cnet.MustParseNetwork("200.0.0.0/24")
		var block1, block2 cnet.IPNet

		BeforeEach(func() {
			bc.Clean()
			deleteAllPoolsWindows()
			applyPoolWindows("100.0.0.0/24", true)
			applyPoolWindows("200.0.0.0/24", true)

			// Hosts must exist before trying to autoassign
			err := applyNode(bc, kc, host, nil)
			Expect(err).NotTo(HaveOccurred())

			setAffinity(ic, true)
		})

		AfterEach(func() {
			setAffinity(ic, false)
			deleteNode(bc, kc, host)
		})

		It("Windows: Should get an IP from pool1 when explicitly requesting from that pool", func() {

			args_1 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              host,
				IPv4Pools:             []cnet.IPNet{pool1},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}

			ctx := context.WithValue(context.Background(), "windowsHost", "windows")
			v4_1, _, outErr := ic.AutoAssign(ctx, args_1)
			blocks := getAffineBlocks(bc, host)
			for _, b := range blocks {
				if pool1.Contains(b.IPNet.IP) {
					block1 = b
				}
			}

			Expect(outErr).NotTo(HaveOccurred())
			Expect(pool1.IPNet.Contains(v4_1[0].IP)).To(BeTrue())

			By("Windows: Should get an IP from pool2 when explicitly requesting from that pool")

			args_2 := AutoAssignArgs{
				Num4:                  1,
				Num6:                  0,
				Hostname:              host,
				IPv4Pools:             []cnet.IPNet{pool2},
				HostReservedAttrIPv4s: rsvdAttrWindows,
			}

			v4_2, _, outErr := ic.AutoAssign(ctx, args_2)
			blocks = getAffineBlocks(bc, host)
			for _, b := range blocks {
				if pool2.Contains(b.IPNet.IP) {
					block2 = b
				}
			}

			Expect(outErr).NotTo(HaveOccurred())
			Expect(block2.IPNet.Contains(v4_2[0].IP)).To(BeTrue())

			By("Windows: Should get an IP from pool1 in the same allocation block as the first IP from pool1")

			v4_3, _, outErr := ic.AutoAssign(ctx, args_1)
			Expect(outErr).NotTo(HaveOccurred())
			Expect(block1.IPNet.Contains(v4_3[0].IP)).To(BeTrue())

			By("Windows: Should get an IP from pool2 in the same allocation block as the first IP from pool2")

			v4_4, _, outErr := ic.AutoAssign(ctx, args_2)
			Expect(outErr).NotTo(HaveOccurred())
			Expect(block2.IPNet.Contains(v4_4[0].IP)).To(BeTrue())

			// Assign the rest of the addresses in pool2.
			// A /24 has 256 addresses and block size is 26 so we would have 16 reserved ips and We've assigned 2 already, so assign (256-18) 238 more.
			args_2.Num4 = 238

			By("Windows: Allocating the rest of the IPs in the pool")
			v4_5, _, outErr := ic.AutoAssign(ctx, args_2)
			Expect(outErr).NotTo(HaveOccurred())
			Expect(len(v4_5)).To(Equal(238))

			// Expect all the IPs to be in pool2.
			for _, a := range v4_5 {
				Expect(pool2.IPNet.Contains(a.IP)).To(BeTrue(), fmt.Sprintf("%s not in pool %s", a.IP, pool2))
			}

			By("Windows: Attempting to allocate an IP when there are no more left in the pool")
			args_2.Num4 = 1
			v4_6, _, outErr := ic.AutoAssign(ctx, args_2)

			Expect(outErr).NotTo(HaveOccurred())
			Expect(len(v4_6)).To(Equal(0))
		})

	})

	DescribeTable("Windows: AutoAssign: requested IPs vs returned IPs",
		func(host string, cleanEnv bool, pools []pool, rsvd *HostReservedAttr, usePool string, inv4, inv6, expv4, expv6 int, expError error) {
			if cleanEnv {
				bc.Clean()
				deleteAllPoolsWindows()
			}

			setAffinity(ic, true)
			defer setAffinity(ic, false)

			for _, v := range pools {
				ipPoolsWindows.pools[v.cidr] = pool{cidr: v.cidr, enabled: v.enabled, blockSize: v.blockSize}
			}

			// Host must exist before trying to autoassign to it
			err := applyNode(bc, kc, host, nil)
			Expect(err).NotTo(HaveOccurred())
			defer deleteNode(bc, kc, host)

			fromPool := cnet.MustParseNetwork(usePool)
			args := AutoAssignArgs{
				Num4:                  inv4,
				Num6:                  inv6,
				Hostname:              host,
				IPv4Pools:             []cnet.IPNet{fromPool},
				HostReservedAttrIPv4s: rsvd,
				HostReservedAttrIPv6s: rsvd,
			}

			ctx := context.WithValue(context.Background(), "windowsHost", "windows")
			outv4, outv6, outErr := ic.AutoAssign(ctx, args)
			if expError != nil {
				Expect(outErr).To(Equal(expError))
			} else {
				Expect(outErr).ToNot(HaveOccurred())
			}
			Expect(outv4).To(HaveLen(expv4))
			Expect(outv6).To(HaveLen(expv6))
		},

		// Test 1: AutoAssign 256 IPv4, 256 IPv6 - expect 240 IPv4 + IPv6 addresses.
		Entry("256 v4 256 v6", "testHost", true, []pool{
			{cidr: "192.168.1.0/24", blockSize: 26, enabled: true},
			{cidr: "fd80:24e2:f998:72d6::/120", blockSize: 122, enabled: true},
		}, rsvdAttrWindows, "192.168.1.0/24", 256, 256, 240, 240, nil),

		// Test 2: AutoAssign 257 IPv4, 0 IPv6 - expect 240 IPv4 addresses, no IPv6, and no error.
		Entry("257 v4 0 v6", "testHost", true, []pool{{cidr: "192.168.1.0/24", blockSize: 26, enabled: true}}, rsvdAttrWindows, "192.168.1.0/24", 257, 0, 240, 0, nil),

		// Test 3: AutoAssign 0 IPv4, 257 IPv6 - expect 240 IPv6 addresses, no IPv4, and no error.
		Entry("0 v4 257 v6", "testHost", true, []pool{
			{cidr: "192.168.1.0/24", blockSize: 26, enabled: true},
			{cidr: "fd80:24e2:f998:72d6::/120", blockSize: 122, enabled: true},
		}, rsvdAttrWindows, "192.168.1.0/24", 0, 257, 0, 240, nil),

		// Test 4: AutoAssign with invalid HostReserveAttr should return error.
		Entry("1 v4 0 v6", "testHost", true, []pool{{cidr: "192.168.1.0/24", blockSize: 26, enabled: true}}, rsvdAttrTooBig, "192.168.1.0/24", 1, 0, 0, 0, ErrNoQualifiedPool),

		Entry("0 v4 1 v6", "testHost", true, []pool{{cidr: "fd80:24e2:f998:72d6::/120", blockSize: 122, enabled: true}}, rsvdAttrTooBig, "fd80:24e2:f998:72d6::/120", 0, 1, 0, 0, ErrNoQualifiedPool),
	)
})

func setAffinity(ic Interface, affinity bool) {
	cfg, err := ic.GetIPAMConfig(context.Background())
	Expect(err).NotTo(HaveOccurred())

	cfg.StrictAffinity = affinity
	err = ic.SetIPAMConfig(context.Background(), *cfg)
	Expect(err).NotTo(HaveOccurred())
}

func deleteAllPoolsWindows() {
	log.Infof("Windows: Deleting all pools")
	ipPoolsWindows.pools = map[string]pool{}
}

func applyPoolWindows(cidr string, enabled bool) {
	log.Infof("Windows: Adding pool: %s, enabled: %v", cidr, enabled)
	ipPoolsWindows.pools[cidr] = pool{enabled: enabled}
}

// checkWindowsIP() receives an IP and block size and returns bool -
// True - if the IP is NOT a reserved IP, i.e, the gateway IP, the first IP, the second IP or the broadcast IP
// False - if the IP is a reserved IP
// This is only handling IPv4
func checkWindowsValidIP(ip net.IP, blockSize uint) bool {
	var mask uint32 = 0xffffffff
	mask = mask >> blockSize
	ipv4 := ip.To4()

	var ipBinary uint32
	ipBinary = 0

	for i := 0; i < 4; i++ {
		ipBinary = ipBinary << 8

		ipBinary = ipBinary | uint32(ipv4[i])
	}
	ipBinary = ipBinary & mask

	if ipBinary == 0x00000000 || ipBinary == 0x00000001 || ipBinary == 0x00000002 || ipBinary == mask {
		return false
	}
	return true
}

// Return boolean after checking if the valid handle is allocated
func isValidWindowsHandle(backend bapi.Client, ipPoolsWindows *ipPoolAccessor, ip net.IP, ctx context.Context) bool {
	c := &ipamClientWindows{
		client: backend,
		pools:  *ipPoolsWindows,
		blockReaderWriter: blockReaderWriter{
			client: backend,
			pools:  ipPoolsWindows,
		},
	}

	ipv4 := cnet.IP{IP: ip}
	blockCIDR := c.GetAssignmentBlockCIDR(ctx, ipv4)
	opts := model.BlockListOptions{IPVersion: 4}
	datastoreObjs, _ := backend.List(context.Background(), opts, "")
	var block allocationBlock
	for _, o := range datastoreObjs.KVPairs {
		k := o.Key.(model.BlockKey)
		if k.CIDR.IP.String() == blockCIDR.IP.String() && k.CIDR.Mask.String() == blockCIDR.Mask.String() {
			block = allocationBlock{o.Value.(*model.AllocationBlock)}
		}

	}

	for _, attrIdx := range block.Allocations {
		if attrIdx == nil {
			continue
		}
		attrs := block.Attributes[*attrIdx]
		// If primary attribute is not nil then it must contain "windows-reserved-IPAM-handle"
		// Primary attribute will be set only for reserved IPs.
		if attrs.AttrPrimary == nil {
			return false
		}
		if *attrs.AttrPrimary == "windows-reserved-ipam-handle" {
			return true
		}
	}

	return false
}
