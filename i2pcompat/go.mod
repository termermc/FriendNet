module friendnet.org/i2pcompat

go 1.26.1

require (
	friendnet.org/protocol v0.0.0
)

replace (
	friendnet.org/protocol => ../protocol
)
