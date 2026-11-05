# Switches are imported by MAC address, which selects the switch on the
# provider's site. Use the controller's dash-separated MAC form.
terraform import omada_switch.core AA-BB-CC-DD-EE-FF

# A switch on another site is imported as "<site>:<switch_mac>", where <site>
# is either the site ID or the site name.
terraform import omada_switch.core Default:AA-BB-CC-DD-EE-FF
