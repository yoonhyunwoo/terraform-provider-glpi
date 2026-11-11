package provider

type kind int

const (
	kindString kind = iota
	kindInt
)

type field struct {
	tf       string
	api      string
	kind     kind
	required bool
	readOnly bool
	desc     string
}

type itemSpec struct {
	itemtype string
	tfName   string
	desc     string
	fields   []field
}

func str(tf, desc string) field { return field{tf: tf, api: tf, kind: kindString, desc: desc} }

func ref(tf, api, desc string) field { return field{tf: tf, api: api, kind: kindInt, desc: desc} }

var nameField = field{tf: "name", api: "name", kind: kindString, required: true, desc: "Name of the item."}

var commentField = str("comment", "Free-form comment.")

var entityField = ref("entity_id", "entities_id", "ID of the entity that owns the item. 0 is the root entity.")

func assetFields(typeAPI, modelAPI string) []field {
	return []field{
		nameField,
		str("serial", "Serial number."),
		str("otherserial", "Inventory number."),
		str("contact", "Contact person."),
		commentField,
		ref("location_id", "locations_id", "ID of the location."),
		ref("manufacturer_id", "manufacturers_id", "ID of the manufacturer."),
		ref("state_id", "states_id", "ID of the status."),
		ref("type_id", typeAPI, "ID of the item type."),
		ref("model_id", modelAPI, "ID of the item model."),
		entityField,
	}
}

var itemSpecs = []itemSpec{
	{
		itemtype: "Computer",
		tfName:   "computer",
		desc:     "A GLPI computer asset.",
		fields:   assetFields("computertypes_id", "computermodels_id"),
	},
	{
		itemtype: "NetworkEquipment",
		tfName:   "network_equipment",
		desc:     "A GLPI network equipment asset (switch, firewall, access point, ...).",
		fields:   assetFields("networkequipmenttypes_id", "networkequipmentmodels_id"),
	},
	{
		itemtype: "Location",
		tfName:   "location",
		desc:     "A GLPI location. Locations form a tree through parent_id.",
		fields: []field{
			nameField,
			commentField,
			ref("parent_id", "locations_id", "ID of the parent location. 0 means top level."),
			entityField,
			{tf: "completename", api: "completename", kind: kindString, readOnly: true, desc: "Full path of the location, computed by GLPI."},
		},
	},
	{
		itemtype: "Manufacturer",
		tfName:   "manufacturer",
		desc:     "A GLPI manufacturer.",
		fields: []field{
			nameField,
			commentField,
			entityField,
		},
	},
}
