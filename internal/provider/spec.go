package provider

import (
	"fmt"
	"net"
	"strconv"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

type kind int

const (
	kindString kind = iota
	kindInt
	kindBool
	kindFloat
)

type field struct {
	tf        string
	api       string
	kind      kind
	required  bool
	readOnly  bool
	immutable bool
	target    string
	desc      string
	// minGLPI is the first GLPI major version that has the column, 0 for every version.
	minGLPI int
}

type itemSpec struct {
	itemtype string
	tfName   string
	desc     string
	// dataOnly specs are exposed as data sources only.
	dataOnly bool
	fields   []field
	// prepare adds the input GLPI derives from several attributes, such as an IP network's CIDR.
	prepare func(in glpi.Item)
}

func (s itemSpec) nameField() (field, bool) {
	for _, f := range s.fields {
		if f.tf == "name" {
			return f, true
		}
	}
	return field{}, false
}

func str(tf, desc string) field { return field{tf: tf, api: tf, kind: kindString, desc: desc} }

func num(tf, desc string) field { return field{tf: tf, api: tf, kind: kindInt, desc: desc} }

func flag(tf, desc string) field { return field{tf: tf, api: tf, kind: kindBool, desc: desc} }

func dec(tf, desc string) field { return field{tf: tf, api: tf, kind: kindFloat, desc: desc} }

func ref(tf, api, target, desc string) field {
	return field{tf: tf, api: api, kind: kindInt, target: target, desc: desc}
}

func (f field) req() field { f.required = true; return f }

func (f field) ro() field { f.readOnly = true; return f }

// fixed marks a field that GLPI cannot change on an existing item, so changing it replaces the resource.
func (f field) fixed() field { f.immutable = true; return f }

func (f field) column(api string) field { f.api = api; return f }

// needs marks a column that only exists from the given GLPI major version on.
func (f field) needs(major int) field { f.minGLPI = major; return f }

func (f field) description() string {
	if f.minGLPI == 0 {
		return f.desc
	}
	return fmt.Sprintf("%s Needs GLPI %d or later.", f.desc, f.minGLPI)
}

var (
	nameField     = str("name", "Name of the item.").req()
	commentField  = str("comment", "Free-form comment.")
	entityField   = ref("entity_id", "entities_id", "Entity", "ID of the entity that owns the item. 0 is the root entity.")
	completeField = str("completename", "Full path of the item in its tree, computed by GLPI.").ro()
)

func assetFields(typeAPI, modelAPI, typeTarget, modelTarget string, extra ...field) []field {
	fields := []field{
		nameField,
		str("serial", "Serial number."),
		str("otherserial", "Inventory number."),
		str("contact", "Contact person."),
		commentField,
		ref("location_id", "locations_id", "Location", "ID of the location."),
		ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the manufacturer."),
		ref("state_id", "states_id", "State", "ID of the status."),
		ref("type_id", typeAPI, typeTarget, "ID of the item type."),
		ref("model_id", modelAPI, modelTarget, "ID of the item model."),
		entityField,
	}
	return append(fields, extra...)
}

func treeFields(parentTF, parentAPI, target string, extra ...field) []field {
	fields := []field{
		nameField,
		commentField,
		ref(parentTF, parentAPI, target, "ID of the parent. 0 means top level."),
		completeField,
	}
	return append(fields, extra...)
}

func simpleFields(extra ...field) []field {
	return append([]field{nameField, commentField}, extra...)
}

var itemSpecs = []itemSpec{
	{
		itemtype: "Computer",
		tfName:   "computer",
		desc:     "GLPI computer asset.",
		fields:   assetFields("computertypes_id", "computermodels_id", "ComputerType", "ComputerModel", str("uuid", "UUID of the machine.")),
	},
	{
		itemtype: "NetworkEquipment",
		tfName:   "network_equipment",
		desc:     "GLPI network equipment asset (switch, firewall, access point).",
		fields:   assetFields("networkequipmenttypes_id", "networkequipmentmodels_id", "NetworkEquipmentType", "NetworkEquipmentModel"),
	},
	{
		itemtype: "Peripheral",
		tfName:   "peripheral",
		desc:     "GLPI peripheral asset.",
		fields:   assetFields("peripheraltypes_id", "peripheralmodels_id", "PeripheralType", "PeripheralModel", str("brand", "Brand of the peripheral.")),
	},
	{
		itemtype: "Location",
		tfName:   "location",
		desc:     "GLPI location.",
		fields: treeFields("parent_id", "locations_id", "Location",
			entityField,
			str("code", "Location code.").needs(11),
			str("alias", "Location alias.").needs(11),
			str("address", "Street address."),
			str("postcode", "Postal code."),
			str("town", "Town."),
			str("state", "State or province."),
			str("country", "Country."),
			str("building", "Building number or name."),
			str("room", "Room number or name."),
			str("latitude", "Latitude."),
			str("longitude", "Longitude."),
			str("altitude", "Altitude."),
		),
	},
	{
		itemtype: "Manufacturer",
		tfName:   "manufacturer",
		desc:     "GLPI manufacturer.",
		fields:   simpleFields(),
	},
	{
		itemtype: "Datacenter",
		tfName:   "datacenter",
		desc:     "GLPI data center.",
		fields: []field{
			nameField,
			ref("location_id", "locations_id", "Location", "ID of the location."),
			entityField,
		},
	},
	{
		itemtype: "DCRoom",
		tfName:   "datacenter_room",
		desc:     "GLPI data center room.",
		fields: []field{
			nameField,
			ref("location_id", "locations_id", "Location", "ID of the location."),
			ref("datacenter_id", "datacenters_id", "Datacenter", "ID of the data center that holds the room."),
			num("columns", "Number of grid columns in the room's rack view.").column("vis_cols"),
			num("rows", "Number of grid rows in the room's rack view.").column("vis_rows"),
			num("cell_width", "Width of a grid cell in the rack view.").column("vis_cell_width").needs(11),
			num("cell_height", "Height of a grid cell in the rack view.").column("vis_cell_height").needs(11),
			entityField,
		},
	},
	{
		itemtype: "Rack",
		tfName:   "rack",
		desc:     "GLPI rack.",
		fields: []field{
			nameField,
			str("serial", "Serial number."),
			str("otherserial", "Inventory number."),
			commentField,
			ref("location_id", "locations_id", "Location", "ID of the location."),
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the manufacturer."),
			ref("state_id", "states_id", "State", "ID of the status."),
			ref("datacenter_room_id", "dcrooms_id", "DCRoom", "ID of the data center room that holds the rack."),
			ref("type_id", "racktypes_id", "RackType", "ID of the rack type."),
			ref("model_id", "rackmodels_id", "RackModel", "ID of the rack model."),
			num("units", "Number of rack units.").column("number_units"),
			num("room_orientation", "Orientation of the rack in the room."),
			str("position", "Position of the rack in the room grid, written as \"column,row\"."),
			str("background_color", "Background color of the rack in the room view, such as #d9ead3.").column("bgcolor"),
			num("max_power", "Maximum power in watts."),
			entityField,
		},
	},
	{
		itemtype: "Item_Rack",
		tfName:   "rack_item",
		desc:     "GLPI rack placement of an item.",
		fields: []field{
			ref("rack_id", "racks_id", "Rack", "ID of the rack.").req().fixed(),
			str("item_type", "GLPI itemtype of the placed item, for example Computer or NetworkEquipment.").column("itemtype").req().fixed(),
			ref("item_id", "items_id", "Computer", "ID of the placed item.").req().fixed(),
			num("position", "Lowest rack unit the item occupies.").req(),
			num("orientation", "0 for the front of the rack, 1 for the rear."),
			str("background_color", "Background color of the item in the rack view.").column("bgcolor"),
			num("horizontal_position", "Position inside a unit shared by half-width items.").column("hpos"),
			flag("reserved", "Reserve the units without an item.").column("is_reserved"),
		},
	},
	{
		itemtype: "State",
		tfName:   "state",
		desc:     "GLPI status, such as In use or Retired.",
		fields: treeFields("parent_id", "states_id", "State",
			entityField,
			flag("helpdesk_visible", "Show the status in the helpdesk.").column("is_helpdesk_visible").needs(11),
		),
	},
	{itemtype: "ComputerType", tfName: "computer_type", desc: "GLPI computer type.", fields: simpleFields()},
	{itemtype: "RackType", tfName: "rack_type", desc: "GLPI rack type.", fields: simpleFields(entityField)},
	{itemtype: "RackModel", tfName: "rack_model", desc: "GLPI rack model.", fields: simpleFields(str("product_number", "Manufacturer product number."))},
	{itemtype: "OperatingSystem", tfName: "operating_system", desc: "GLPI operating system.", fields: simpleFields()},
	{itemtype: "OperatingSystemVersion", tfName: "operating_system_version", desc: "GLPI operating system version.", fields: simpleFields()},
	{itemtype: "Network", tfName: "network", desc: "GLPI network.", fields: simpleFields()},
	{
		itemtype: "Group",
		tfName:   "group",
		desc:     "GLPI group.",
		fields: treeFields("parent_id", "groups_id", "Group",
			entityField,
			str("code", "Group code.").needs(11),
			flag("requester", "Members can be ticket requesters.").column("is_requester"),
			flag("watcher", "Members can watch tickets.").column("is_watcher"),
			flag("assign", "Tickets can be assigned to the group.").column("is_assign"),
			flag("task", "The group can be assigned tasks.").column("is_task"),
			flag("notify", "The group can be notified.").column("is_notify"),
			flag("item_group", "The group can own items.").column("is_itemgroup"),
			flag("user_group", "The group can contain users.").column("is_usergroup"),
			flag("manager", "The group can manage other groups.").column("is_manager"),
		),
	},
	{
		itemtype: "SoftwareCategory",
		tfName:   "software_category",
		desc:     "GLPI software category.",
		fields:   treeFields("parent_id", "softwarecategories_id", "SoftwareCategory"),
	},
	{
		itemtype: "Software",
		tfName:   "software",
		desc:     "GLPI software.",
		fields: []field{
			nameField,
			commentField,
			ref("location_id", "locations_id", "Location", "ID of the location."),
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the publisher."),
			ref("category_id", "softwarecategories_id", "SoftwareCategory", "ID of the software category."),
			flag("helpdesk_visible", "Show the software in the helpdesk.").column("is_helpdesk_visible"),
			flag("update", "The software is an update of another software.").column("is_update"),
			flag("valid", "Whether the licenses cover the installations, computed by GLPI.").column("is_valid").ro(),
			entityField,
		},
	},
	{
		itemtype: "SoftwareVersion",
		tfName:   "software_version",
		desc:     "GLPI software version.",
		fields: []field{
			nameField,
			commentField,
			ref("software_id", "softwares_id", "Software", "ID of the software.").req().fixed(),
			ref("state_id", "states_id", "State", "ID of the status."),
			ref("operating_system_id", "operatingsystems_id", "OperatingSystem", "ID of the operating system."),
			str("architecture", "Architecture, such as x64.").column("arch"),
			entityField,
		},
	},
	{
		itemtype: "SoftwareLicenseType",
		tfName:   "software_license_type",
		desc:     "GLPI software license type.",
		fields:   treeFields("parent_id", "softwarelicensetypes_id", "SoftwareLicenseType"),
	},
	{
		itemtype: "SoftwareLicense",
		tfName:   "software_license",
		desc:     "GLPI software license.",
		fields: []field{
			nameField,
			commentField,
			str("serial", "License serial number."),
			str("otherserial", "Inventory number."),
			ref("software_id", "softwares_id", "Software", "ID of the software.").req().fixed(),
			ref("type_id", "softwarelicensetypes_id", "SoftwareLicenseType", "ID of the license type."),
			num("number", "Number of installations the license covers. -1 means unlimited."),
			ref("purchased_version_id", "softwareversions_id_buy", "SoftwareVersion", "ID of the purchased software version."),
			ref("used_version_id", "softwareversions_id_use", "SoftwareVersion", "ID of the software version in use."),
			str("expire", "Expiration date as YYYY-MM-DD."),
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the publisher."),
			ref("state_id", "states_id", "State", "ID of the status."),
			ref("location_id", "locations_id", "Location", "ID of the location."),
			flag("valid", "Whether the license is valid, computed by GLPI.").column("is_valid").ro(),
			entityField,
		},
	},
	{
		itemtype: "Infocom",
		tfName:   "infocom",
		desc:     "GLPI financial record of an item: purchase, warranty and depreciation.",
		fields: []field{
			str("item_type", "GLPI itemtype the information belongs to, for example Computer or Rack.").column("itemtype").req().fixed(),
			ref("item_id", "items_id", "Computer", "ID of the item the information belongs to.").req().fixed(),
			str("buy_date", "Purchase date as YYYY-MM-DD."),
			str("use_date", "Date the item entered service as YYYY-MM-DD."),
			dec("value", "Purchase value."),
			dec("warranty_value", "Warranty value."),
			num("warranty_duration", "Warranty duration in months."),
			num("sink_time", "Depreciation period in years."),
			num("sink_type", "Depreciation method as GLPI numbers it: 0 none, 2 linear, 3 degressive."),
			dec("sink_coefficient", "Degressive depreciation coefficient.").column("sink_coeff"),
			str("order_number", "Order number."),
			str("delivery_number", "Delivery number."),
			str("immo_number", "Asset number in the accounting system."),
			str("bill", "Invoice number."),
			commentField,
			entityField,
		},
	},
	{
		itemtype: "IPNetwork",
		tfName:   "ip_network",
		desc:     "GLPI IP network.",
		prepare:  addCIDR,
		fields: []field{
			nameField,
			commentField,
			ref("parent_id", "ipnetworks_id", "IPNetwork", "ID of the smallest IP network that contains this one, chosen by GLPI from the address. 0 means top level.").ro(),
			completeField,
			str("address", "Network address, for example 192.168.50.0.").req(),
			str("netmask", "Netmask, for example 255.255.254.0.").req(),
			str("gateway", "Gateway address."),
			flag("addressable", "Whether addresses in the network are addressable."),
			num("ip_version", "IP version, 4 or 6, computed by GLPI.").column("version").ro(),
			entityField,
		},
	},
	{
		itemtype: "NetworkName",
		tfName:   "network_name",
		desc:     "GLPI network name.",
		prepare:  detachedByDefault,
		fields: []field{
			nameField,
			commentField,
			ref("ip_network_id", "ipnetworks_id", "IPNetwork", "ID of the IP network."),
			str("item_type", "GLPI itemtype of the item that owns the network name.").column("itemtype"),
			ref("item_id", "items_id", "Computer", "ID of the item that owns the network name."),
			entityField,
		},
	},
	{
		itemtype: "IPAddress",
		tfName:   "ip_address",
		desc:     "GLPI IP address.",
		fields: []field{
			str("name", "IP address, for example 10.77.0.254.").req(),
			str("item_type", "GLPI itemtype of the item that owns the address, usually NetworkName.").column("itemtype"),
			ref("item_id", "items_id", "NetworkName", "ID of the item that owns the address."),
			num("ip_version", "IP version, 4 or 6, computed by GLPI.").column("version").ro(),
			entityField,
		},
	},
	{
		itemtype: "User",
		tfName:   "user",
		desc:     "GLPI user.",
		dataOnly: true,
		fields: []field{
			str("name", "Login name.").req(),
			str("realname", "Last name."),
			str("firstname", "First name."),
			commentField,
			flag("active", "Whether the user can log in.").column("is_active"),
		},
	},
	{
		itemtype: "Entity",
		tfName:   "entity",
		desc:     "GLPI entity.",
		dataOnly: true,
		fields:   []field{nameField, commentField, completeField},
	},
	{
		itemtype: "DeviceProcessor",
		tfName:   "device_processor",
		desc:     "GLPI processor model in the device catalog.",
		dataOnly: true,
		fields: []field{
			str("name", "Designation of the processor.").column("designation").req(),
			commentField,
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the manufacturer."),
			num("frequency", "Frequency in MHz.").column("frequence"),
			num("default_frequency", "Default frequency in MHz.").column("frequency_default"),
			num("default_cores", "Default number of cores.").column("nbcores_default"),
			num("default_threads", "Default number of threads.").column("nbthreads_default"),
		},
	},
	{
		itemtype: "DeviceMemory",
		tfName:   "device_memory",
		desc:     "GLPI memory module model in the device catalog.",
		dataOnly: true,
		fields: []field{
			str("name", "Designation of the memory module.").column("designation").req(),
			commentField,
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the manufacturer."),
			num("default_size", "Default size in MB.").column("size_default"),
			num("frequency", "Frequency in MHz.").column("frequence"),
		},
	},
	{
		itemtype: "DeviceGraphicCard",
		tfName:   "device_graphic_card",
		desc:     "GLPI graphic card model in the device catalog.",
		dataOnly: true,
		fields: []field{
			str("name", "Designation of the graphic card.").column("designation").req(),
			commentField,
			ref("manufacturer_id", "manufacturers_id", "Manufacturer", "ID of the manufacturer."),
			str("chipset", "Chipset."),
			num("default_memory", "Default memory in MB.").column("memory_default"),
		},
	},
}

// addCIDR sends the "network" property GLPI requires when it creates or moves an IP network. The
// attributes users write are the address and the netmask, which is also what GLPI stores and returns.
func addCIDR(in glpi.Item) {
	address, _ := in["address"].(string)
	netmask, _ := in["netmask"].(string)
	mask := net.ParseIP(netmask)
	if v4 := mask.To4(); v4 != nil {
		mask = v4
	}
	if address == "" || mask == nil {
		return
	}
	ones, _ := net.IPMask(mask).Size()
	in["network"] = address + "/" + strconv.Itoa(ones)
}

// detachedByDefault fills the owner columns GLPI refuses to leave without a value, so a network name
// can exist before an item uses it.
func detachedByDefault(in glpi.Item) {
	if _, ok := in["itemtype"]; !ok {
		in["itemtype"] = ""
	}
	if _, ok := in["items_id"]; !ok {
		in["items_id"] = 0
	}
}
