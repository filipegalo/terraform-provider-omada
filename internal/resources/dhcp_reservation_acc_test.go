package resources_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/filipegalo/terraform-provider-omada/internal/provider"
)

// This is an acceptance test suite: it talks to a real Omada Controller and
// only runs when TF_ACC is set, per the standard terraform-plugin-testing
// convention (resource.Test skips itself otherwise). Beyond TF_ACC, it needs
// the same credentials the provider itself does (OMADA_URL, OMADA_USERNAME,
// OMADA_PASSWORD, OMADA_SITE) plus OMADA_TEST_NETWORK_ID, the ID of a LAN
// network on that site to reserve an address on. site_id is deliberately
// left unset in the test config so this also exercises the resource's
// default-to-the-provider's-site behavior.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"omada": providerserver.NewProtocol6WithError(provider.New("acctest")()),
}

// testAccPreCheck asserts the provider's own credentials are present, plus any
// variables the calling test needs on top of them.
func testAccPreCheck(t *testing.T, extra ...string) {
	t.Helper()
	for _, envVar := range append([]string{"OMADA_URL", "OMADA_USERNAME", "OMADA_PASSWORD", "OMADA_SITE"}, extra...) {
		if os.Getenv(envVar) == "" {
			t.Fatalf("%s must be set for acceptance tests", envVar)
		}
	}
}

func TestAccDHCPReservation_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t, "OMADA_TEST_NETWORK_ID") },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDHCPReservationConfig("aa:bb:cc:dd:ee:ff", "10.0.0.222", "created by acceptance test", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "mac_address", "aa:bb:cc:dd:ee:ff"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip_address", "10.0.0.222"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "description", "created by acceptance test"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("omada_dhcp_reservation.test", "id"),
					resource.TestCheckResourceAttrSet("omada_dhcp_reservation.test", "site_id"),
				),
			},
			{
				// Confirm ip_address/description/enabled patch in place
				// without replacing the resource.
				Config: testAccDHCPReservationConfig("aa:bb:cc:dd:ee:ff", "10.0.0.223", "updated by acceptance test", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "ip_address", "10.0.0.223"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "description", "updated by acceptance test"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.test", "enabled", "false"),
				),
			},
			{
				ResourceName:      "omada_dhcp_reservation.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccDHCPReservationConfig(mac, ip, description string, enabled bool) string {
	return fmt.Sprintf(`
provider "omada" {}

resource "omada_dhcp_reservation" "test" {
  network_id  = %q
  mac_address = %q
  ip_address  = %q
  description = %q
  enabled     = %t
}
`, os.Getenv("OMADA_TEST_NETWORK_ID"), mac, ip, description, enabled)
}
