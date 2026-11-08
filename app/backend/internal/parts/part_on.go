package parts

// PartOn reports whether part p is on; a nil Control has every part on.
func PartOn(c *Control, p Part) bool { return c == nil || c.On(p) }
