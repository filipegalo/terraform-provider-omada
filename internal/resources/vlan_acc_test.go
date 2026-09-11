package resources_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests create and destroy a real VLAN on a real gateway. Beyond the
// provider's own credentials they need OMADA_TEST_GATEWAY_MAC (the gateway
// that will own the VLAN). OMADA_TEST_VLAN_ID and OMADA_TEST_VLAN_SUBNET
// override the defaults below -- set them if 4094 or 10.254.254.0/24 collide
// with anything real on the site under test.
const (
	defaultTestVLANID = "4094"
	defaultTestSubnet = "10.254.254"
)

func testVLANID() string {
	if v := os.Getenv("OMADA_TEST_VLAN_ID"); v != "" {
		return v
	}
	return defaultTestVLANID
}

func testVLANSubnet() string {
	if v := os.Getenv("OMADA_TEST_VLAN_SUBNET"); v != "" {
		return v
	}
	return defaultTestSubnet
}

func TestAccVLAN_basic(t *testing.T) {
	subnet := testVLANSubnet()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t, "OMADA_TEST_GATEWAY_MAC") },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVLANConfig("acctest", 1440, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_vlan.test", "name", "acctest"),
					resource.TestCheckResourceAttr("omada_vlan.test", "vlan_id", testVLANID()),
					resource.TestCheckResourceAttr("omada_vlan.test", "gateway_subnet", subnet+".1/24"),
					resource.TestCheckResourceAttr("omada_vlan.test", "dhcp_range_start", subnet+".100"),
					resource.TestCheckResourceAttr("omada_vlan.test", "dhcp_lease_time", "1440"),
					resource.TestCheckResourceAttr("omada_vlan.test", "isolation", "false"),
					// Defaulted attributes must land in state, not stay null.
					resource.TestCheckResourceAttr("omada_vlan.test", "dhcp_enabled", "true"),
					resource.TestCheckResourceAttr("omada_vlan.test", "dhcp_dns_mode", "auto"),
					resource.TestCheckResourceAttrSet("omada_vlan.test", "id"),
					resource.TestCheckResourceAttrSet("omada_vlan.test", "site_id"),
				),
			},
			{
				// Import must hydrate every attribute; if it does not, the
				// next plan would propose replacing a live network.
				ResourceName:      "omada_vlan.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["omada_vlan.test"]
					if !ok {
						return "", fmt.Errorf("omada_vlan.test not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["site_id"], rs.Primary.ID), nil
				},
			},
			{
				Config: testAccVLANConfig("acctest-renamed", 720, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_vlan.test", "name", "acctest-renamed"),
					resource.TestCheckResourceAttr("omada_vlan.test", "dhcp_lease_time", "720"),
					resource.TestCheckResourceAttr("omada_vlan.test", "isolation", "true"),
					// An update must not rebind the VLAN to other ports.
					resource.TestCheckResourceAttr("omada_vlan.test", "vlan_id", testVLANID()),
				),
			},
		},
	})
}

// TestAccVLAN_invalidConfig checks the schema and config validators reject bad
// input during plan, without reaching the controller.
func TestAccVLAN_invalidConfig(t *testing.T) {
	cases := map[string]struct {
		config string
		expect string
	}{
		"vlan id out of range": {
			config: testAccVLANConfigRaw(`vlan_id = 5000`),
			expect: `value must be between 1 and 4094`,
		},
		"dhcp enabled without a pool": {
			config: testAccVLANConfigRaw(`vlan_id = 12` + "\n" + `dhcp_enabled = true`),
			expect: `dhcp_range_start is required when dhcp_enabled is true`,
		},
		"unknown dns mode": {
			config: testAccVLANConfigRaw(`vlan_id = 13` + "\n" + `dhcp_enabled = false` + "\n" + `dhcp_dns_mode = "banana"`),
			expect: `value must be one of`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t, "OMADA_TEST_GATEWAY_MAC") },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      tc.config,
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(regexp.QuoteMeta(tc.expect)),
				}},
			})
		})
	}
}

func testAccVLANConfig(name string, leaseTime int, isolation bool) string {
	subnet := testVLANSubnet()
	return fmt.Sprintf(`
resource "omada_vlan" "test" {
  name             = %q
  vlan_id          = %s
  device_mac       = %q
  gateway_subnet   = "%s.1/24"
  dhcp_range_start = "%s.100"
  dhcp_range_end   = "%s.199"
  dhcp_lease_time  = %d
  isolation        = %t
}
`, name, testVLANID(), os.Getenv("OMADA_TEST_GATEWAY_MAC"), subnet, subnet, subnet, leaseTime, isolation)
}

// testAccVLANConfigRaw builds a VLAN whose invalid parts are supplied verbatim,
// for the validator cases.
func testAccVLANConfigRaw(extra string) string {
	subnet := testVLANSubnet()
	return fmt.Sprintf(`
resource "omada_vlan" "test" {
  name           = "acctest-invalid"
  device_mac     = %q
  gateway_subnet = "%s.1/24"
  %s
}
`, os.Getenv("OMADA_TEST_GATEWAY_MAC"), subnet, extra)
}
