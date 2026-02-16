module friendnet.org/rpcclient

go 1.25.7

require (
	friendnet.org/protocol v0.0.0
)

replace (
	"friendnet.org/protocol" => "../protocol"
)
