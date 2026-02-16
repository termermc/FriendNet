package rpcclient

import (
	"context"
	"fmt"
	"os"
	"strings"

	v1 "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"github.com/chzyer/readline"
)

type Cmd struct {
	Name    string
	Usage   string
	Handler func(cli *Cli, args []string) error
}

// Cli is a command-line interface for the server RPC service.
type Cli struct {
	client   serverrpcv1connect.ServerRpcServiceClient
	commands []Cmd
}

func NewCli(client serverrpcv1connect.ServerRpcServiceClient) *Cli {
	cli := &Cli{client: client}
	cli.commands = []Cmd{
		{
			Name:  "help",
			Usage: "help [command]",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdHelp(args)
			},
		},
		{
			Name:  "getrooms",
			Usage: "getrooms",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdGetRooms(args)
			},
		},
		{
			Name:  "getroominfo",
			Usage: "getroominfo <room>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdGetRoomInfo(args)
			},
		},
		{
			Name:  "getonlineusers",
			Usage: "getonlineusers <room>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdGetOnlineUsers(args)
			},
		},
		{
			Name:  "getonlineuserinfo",
			Usage: "getonlineuserinfo <room> <username>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdGetOnlineUserInfo(args)
			},
		},
		{
			Name:  "getaccounts",
			Usage: "getaccounts <room>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdGetAccounts(args)
			},
		},
		{
			Name:  "createroom",
			Usage: "createroom <room>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdCreateRoom(args)
			},
		},
		{
			Name:  "deleteroom",
			Usage: "deleteroom <room>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdDeleteRoom(args)
			},
		},
		{
			Name:  "createaccount",
			Usage: "createaccount <room> <username> [password]",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdCreateAccount(args)
			},
		},
		{
			Name:  "deleteaccount",
			Usage: "deleteaccount <room> <username>",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdDeleteAccount(args)
			},
		},
		{
			Name:  "updateaccountpassword",
			Usage: "updateaccountpassword <room> <username> [password]",
			Handler: func(cli *Cli, args []string) error {
				return cli.cmdUpdateAccountPassword(args)
			},
		},
	}
	return cli
}

func (c *Cli) Do(cmdStr string) error {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		// Empty command.
		return nil
	}

	name := parts[0]
	for _, cmd := range c.commands {
		if cmd.Name == name {
			return cmd.Handler(c, parts[1:])
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %q. Type \"help\" to see a list of commands.\n", name)

	return nil
}

func (c *Cli) cmdHelp(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: help [command]")
	}
	if len(args) == 1 {
		name := args[0]
		for _, cmd := range c.commands {
			if cmd.Name == name {
				fmt.Printf("%s - %s\n", cmd.Name, cmd.Usage)
				return nil
			}
		}
		return fmt.Errorf("unknown command: %q", name)
	}

	var maxLen int
	for _, cmd := range c.commands {
		if len(cmd.Name) > maxLen {
			maxLen = len(cmd.Name)
		}
	}

	fmt.Println("Commands:")
	for _, cmd := range c.commands {
		fmt.Printf("  %-*s  %s\n", maxLen, cmd.Name, cmd.Usage)
	}
	return nil
}

func (c *Cli) cmdGetRooms(args []string) error {
	if err := validateArgCount(args, 0, 0, "getrooms"); err != nil {
		return err
	}

	resp, err := c.client.GetRooms(context.Background(), &v1.GetRoomsRequest{})
	if err != nil {
		return err
	}

	rooms := resp.GetRooms()
	if len(rooms) == 0 {
		fmt.Println("No rooms.")
		return nil
	}
	for _, room := range rooms {
		if room == nil {
			continue
		}
		fmt.Printf("%s (online users: %d)\n", room.GetName(), room.GetOnlineUserCount())
	}
	return nil
}

func (c *Cli) cmdGetRoomInfo(args []string) error {
	if err := validateArgCount(args, 1, 1, "getroominfo <room>"); err != nil {
		return err
	}

	resp, err := c.client.GetRoomInfo(context.Background(), &v1.GetRoomInfoRequest{
		Name: args[0],
	})
	if err != nil {
		return err
	}

	room := resp.GetRoom()
	if room == nil {
		fmt.Println("No room info returned.")
		return nil
	}
	fmt.Printf("%s (online users: %d)\n", room.GetName(), room.GetOnlineUserCount())
	return nil
}

func (c *Cli) cmdGetOnlineUsers(args []string) error {
	if err := validateArgCount(args, 1, 1, "getonlineusers <room>"); err != nil {
		return err
	}

	stream, err := c.client.GetOnlineUsers(context.Background(), &v1.GetOnlineUsersRequest{
		Room: args[0],
	})
	if err != nil {
		return err
	}

	var count int
	for stream.Receive() {
		msg := stream.Msg()
		for _, user := range msg.GetUsers() {
			if user == nil {
				continue
			}
			fmt.Println(user.GetUsername())
			count++
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	if count == 0 {
		fmt.Println("No online users.")
	}
	return nil
}

func (c *Cli) cmdGetOnlineUserInfo(args []string) error {
	if err := validateArgCount(args, 2, 2, "getonlineuserinfo <room> <username>"); err != nil {
		return err
	}

	resp, err := c.client.GetOnlineUserInfo(context.Background(), &v1.GetOnlineUserInfoRequest{
		Room:     args[0],
		Username: args[1],
	})
	if err != nil {
		return err
	}

	user := resp.GetUser()
	if user == nil {
		fmt.Println("No user info returned.")
		return nil
	}
	fmt.Println(user.GetUsername())
	return nil
}

func (c *Cli) cmdGetAccounts(args []string) error {
	if err := validateArgCount(args, 1, 1, "getaccounts <room>"); err != nil {
		return err
	}

	resp, err := c.client.GetAccounts(context.Background(), &v1.GetAccountsRequest{
		Room: args[0],
	})
	if err != nil {
		return err
	}

	accounts := resp.GetAccounts()
	if len(accounts) == 0 {
		fmt.Println("No accounts.")
		return nil
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		fmt.Println(account.GetUsername())
	}
	return nil
}

func (c *Cli) cmdCreateRoom(args []string) error {
	if err := validateArgCount(args, 1, 1, "createroom <room>"); err != nil {
		return err
	}

	resp, err := c.client.CreateRoom(context.Background(), &v1.CreateRoomRequest{
		Name: args[0],
	})
	if err != nil {
		return err
	}

	room := resp.GetRoom()
	if room == nil {
		fmt.Printf("Room %q created.\n", args[0])
		return nil
	}
	fmt.Printf("Created room %s (online users: %d)\n", room.GetName(), room.GetOnlineUserCount())
	return nil
}

func (c *Cli) cmdDeleteRoom(args []string) error {
	if err := validateArgCount(args, 1, 1, "deleteroom <room>"); err != nil {
		return err
	}

	_, err := c.client.DeleteRoom(context.Background(), &v1.DeleteRoomRequest{
		Name: args[0],
	})
	if err != nil {
		return err
	}

	fmt.Printf("Deleted room %q.\n", args[0])
	return nil
}

func (c *Cli) cmdCreateAccount(args []string) error {
	if err := validateArgCount(args, 2, 3, "createaccount <room> <username> [password]"); err != nil {
		return err
	}

	pass := ""
	if len(args) == 3 {
		pass = args[2]
	}

	resp, err := c.client.CreateAccount(context.Background(), &v1.CreateAccountRequest{
		Room:     args[0],
		Username: args[1],
		Password: pass,
	})
	if err != nil {
		return err
	}

	if gen := resp.GetGeneratedPassword(); gen != "" {
		fmt.Printf("Generated password: %s\n", gen)
	} else {
		fmt.Printf("Account %q created in room %q.\n", args[1], args[0])
	}
	return nil
}

func (c *Cli) cmdDeleteAccount(args []string) error {
	if err := validateArgCount(args, 2, 2, "deleteaccount <room> <username>"); err != nil {
		return err
	}

	_, err := c.client.DeleteAccount(context.Background(), &v1.DeleteAccountRequest{
		Room:     args[0],
		Username: args[1],
	})
	if err != nil {
		return err
	}

	fmt.Printf("Deleted account %q in room %q.\n", args[1], args[0])
	return nil
}

func (c *Cli) cmdUpdateAccountPassword(args []string) error {
	if err := validateArgCount(args, 2, 3, "updateaccountpassword <room> <username> [password]"); err != nil {
		return err
	}

	pass := ""
	if len(args) == 3 {
		pass = args[2]
	}

	resp, err := c.client.UpdateAccountPassword(context.Background(), &v1.UpdateAccountPasswordRequest{
		Room:     args[0],
		Username: args[1],
		Password: pass,
	})
	if err != nil {
		return err
	}

	if resp == nil {
		fmt.Printf("Updated password for %q in room %q.\n", args[1], args[0])
		return nil
	}
	if gen := resp.GetGeneratedPassword(); gen != "" {
		fmt.Printf("Generated password: %s\n", gen)
	} else {
		fmt.Printf("Updated password for %q in room %q.\n", args[1], args[0])
	}
	return nil
}

func validateArgCount(args []string, min int, max int, usage string) error {
	if len(args) < min {
		return fmt.Errorf("usage: %s", usage)
	}
	if max >= 0 && len(args) > max {
		return fmt.Errorf("usage: %s", usage)
	}
	return nil
}

// Run runs the CLI.
// It returns when the client presses CTRL+D.
func (c *Cli) Run() {
	println("Welcome to the FriendNet server RPC CLI.\nType \"help\" for a list of commands.")
	rl, newErr := readline.NewEx(&readline.Config{
		Prompt:       "> ",
		AutoComplete: c.completer(),
	})
	if newErr != nil {
		panic(newErr)
	}
	defer func() {
		_ = rl.Close()
	}()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		doErr := c.Do(line)
		if doErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, doErr.Error()+"\n")
		}
	}
}

func (c *Cli) completer() readline.AutoCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(c.commands))
	helpChildren := make([]readline.PrefixCompleterInterface, 0, len(c.commands))
	for _, cmd := range c.commands {
		if cmd.Name == "help" {
			continue
		}
		helpChildren = append(helpChildren, readline.PcItem(cmd.Name))
	}
	items = append(items, readline.PcItem("help", helpChildren...))
	for _, cmd := range c.commands {
		if cmd.Name == "help" {
			continue
		}
		items = append(items, readline.PcItem(cmd.Name))
	}
	return readline.NewPrefixCompleter(items...)
}
