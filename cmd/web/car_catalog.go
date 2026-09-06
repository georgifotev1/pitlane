package main

// The built-in catalogue covers makes and models commonly seen in Bulgarian
// garages. It only supplies suggestions: owners can always enter another value.
var commonCarMakes = []string{
	"Alfa Romeo",
	"Audi",
	"BMW",
	"Chevrolet",
	"Chrysler",
	"Citroen",
	"Cupra",
	"Dacia",
	"Daewoo",
	"Fiat",
	"Ford",
	"Great Wall",
	"Honda",
	"Hyundai",
	"Isuzu",
	"Iveco",
	"Jaguar",
	"Jeep",
	"Kia",
	"Lada",
	"Land Rover",
	"Lexus",
	"Mazda",
	"Mercedes-Benz",
	"MINI",
	"Mitsubishi",
	"Nissan",
	"Opel",
	"Peugeot",
	"Porsche",
	"Renault",
	"Saab",
	"Seat",
	"Skoda",
	"Smart",
	"SsangYong",
	"Subaru",
	"Suzuki",
	"Tesla",
	"Toyota",
	"Volkswagen",
	"Volvo",
}

type carModelSuggestion struct {
	Make  string
	Model string
}

var commonCarModels = modelSuggestions(map[string][]string{
	"Alfa Romeo":    {"147", "156", "159", "Giulia", "Giulietta", "Stelvio"},
	"Audi":          {"A3", "A4", "A5", "A6", "A8", "Q3", "Q5", "Q7"},
	"BMW":           {"1 Series", "3 Series", "5 Series", "7 Series", "X1", "X3", "X5", "X6"},
	"Chevrolet":     {"Aveo", "Captiva", "Cruze", "Lacetti", "Orlando", "Spark"},
	"Chrysler":      {"300C", "Grand Voyager", "PT Cruiser", "Voyager"},
	"Citroen":       {"Berlingo", "C3", "C4", "C5", "C-Elysee", "Jumper", "Jumpy", "Xsara"},
	"Cupra":         {"Ateca", "Born", "Formentor", "Leon"},
	"Dacia":         {"Dokker", "Duster", "Jogger", "Logan", "Sandero"},
	"Daewoo":        {"Kalos", "Lanos", "Leganza", "Matiz", "Nubira"},
	"Fiat":          {"500", "Bravo", "Doblo", "Ducato", "Grande Punto", "Panda", "Punto", "Tipo"},
	"Ford":          {"C-Max", "Fiesta", "Focus", "Galaxy", "Kuga", "Mondeo", "S-Max", "Transit"},
	"Great Wall":    {"Hover", "Steed", "Voleex C10", "Voleex C30"},
	"Honda":         {"Accord", "Civic", "CR-V", "FR-V", "HR-V", "Jazz"},
	"Hyundai":       {"i10", "i20", "i30", "i40", "Kona", "Santa Fe", "Tucson"},
	"Isuzu":         {"D-Max", "Trooper"},
	"Iveco":         {"Daily"},
	"Jaguar":        {"E-Pace", "F-Pace", "XE", "XF", "X-Type"},
	"Jeep":          {"Cherokee", "Compass", "Grand Cherokee", "Renegade", "Wrangler"},
	"Kia":           {"Carens", "Ceed", "Niro", "Picanto", "Rio", "Sorento", "Sportage", "Stonic"},
	"Lada":          {"2105", "2107", "Niva", "Samara"},
	"Land Rover":    {"Defender", "Discovery", "Freelander", "Range Rover", "Range Rover Evoque", "Range Rover Sport"},
	"Lexus":         {"CT", "ES", "GS", "IS", "NX", "RX"},
	"Mazda":         {"2", "3", "5", "6", "CX-3", "CX-5", "CX-7"},
	"Mercedes-Benz": {"A-Class", "B-Class", "C-Class", "CLA", "CLS", "E-Class", "GLA", "GLC", "GLE", "S-Class", "Sprinter", "Vito"},
	"MINI":          {"Clubman", "Countryman", "Hatch"},
	"Mitsubishi":    {"ASX", "Carisma", "Colt", "L200", "Lancer", "Outlander", "Pajero", "Space Star"},
	"Nissan":        {"Almera", "Juke", "Leaf", "Micra", "Navara", "Note", "Pathfinder", "Primera", "Qashqai", "X-Trail"},
	"Opel":          {"Astra", "Corsa", "Insignia", "Meriva", "Mokka", "Omega", "Signum", "Vectra", "Vivaro", "Zafira"},
	"Peugeot":       {"107", "206", "207", "208", "307", "308", "407", "508", "2008", "3008", "Boxer", "Partner"},
	"Porsche":       {"Cayenne", "Macan", "Panamera"},
	"Renault":       {"Captur", "Clio", "Espace", "Kangoo", "Laguna", "Megane", "Scenic", "Talisman", "Trafic"},
	"Saab":          {"9-3", "9-5"},
	"Seat":          {"Alhambra", "Altea", "Arona", "Ateca", "Ibiza", "Leon", "Toledo"},
	"Skoda":         {"Citigo", "Fabia", "Kamiq", "Karoq", "Kodiaq", "Octavia", "Rapid", "Roomster", "Superb", "Yeti"},
	"Smart":         {"Forfour", "Fortwo"},
	"SsangYong":     {"Korando", "Kyron", "Musso", "Rexton"},
	"Subaru":        {"Forester", "Impreza", "Legacy", "Outback", "XV"},
	"Suzuki":        {"Baleno", "Grand Vitara", "Ignis", "Jimny", "S-Cross", "Swift", "Vitara"},
	"Tesla":         {"Model 3", "Model S", "Model X", "Model Y"},
	"Toyota":        {"Auris", "Avensis", "Aygo", "C-HR", "Camry", "Corolla", "Hilux", "Land Cruiser", "Prius", "RAV4", "Yaris"},
	"Volkswagen":    {"Bora", "Caddy", "Crafter", "Golf", "Jetta", "Passat", "Polo", "Sharan", "Tiguan", "Touareg", "Touran", "Transporter"},
	"Volvo":         {"C30", "S40", "S60", "S80", "S90", "V40", "V50", "V60", "V70", "XC40", "XC60", "XC70", "XC90"},
})

// modelSuggestions follows commonCarMakes rather than map iteration order so
// the server-rendered options and tests remain deterministic.
func modelSuggestions(catalog map[string][]string) []carModelSuggestion {
	var suggestions []carModelSuggestion
	for _, makeName := range commonCarMakes {
		for _, model := range catalog[makeName] {
			suggestions = append(suggestions, carModelSuggestion{Make: makeName, Model: model})
		}
	}
	return suggestions
}
