# Reservations are imported as "<site>:<mac_address>", where <site> is either
# the site ID or the site name. The MAC may use either separator.
terraform import omada_dhcp_reservation.printer Default:AA:BB:CC:DD:EE:FF
