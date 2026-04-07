package rpcclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"connectrpc.com/connect"
	v1 "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"github.com/chzyer/readline"
)

// Opt is a function that configures a CLI.
type Opt func(*Cli)

// WithHeaders sets the headers to send along with RPC requests.
func WithHeaders(headers http.Header) Opt {
	return func(cli *Cli) {
		cli.headers = headers
	}
}

// WithWelcomeMsg sets the welcome message to print when the CLI starts.
// An empty string will use the default.
func WithWelcomeMsg(msg string) Opt {
	return func(cli *Cli) {
		cli.welcomeMsg = msg
	}
}

type Cmd struct {
	Name    string
	Usage   string
	Handler func(ctx context.Context, cli *Cli, args []string) error
}

// Cli is a command-line interface for the server RPC service.
type Cli struct {
	client     serverrpcv1connect.ServerRpcServiceClient
	headers    http.Header
	welcomeMsg string
	commands   []Cmd
}

var errStop = errors.New("stop")

// NewCli creates a new CLI.
func NewCli(client serverrpcv1connect.ServerRpcServiceClient, opts ...Opt) *Cli {
	cli := &Cli{
		client: client,
	}
	for _, opt := range opts {
		opt(cli)
	}

	if cli.headers == nil {
		cli.headers = make(http.Header)
	}

	cli.commands = []Cmd{
		{
			Name:  "help",
			Usage: "help [command]",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdHelp(ctx, args)
			},
		},
		{
			Name:  "exit",
			Usage: "exit",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdExit(ctx, args)
			},
		},
		{
			Name:  "getrooms",
			Usage: "getrooms",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdGetRooms(ctx, args)
			},
		},
		{
			Name:  "getroominfo",
			Usage: "getroominfo <room>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdGetRoomInfo(ctx, args)
			},
		},
		{
			Name:  "getonlineusers",
			Usage: "getonlineusers <room>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdGetOnlineUsers(ctx, args)
			},
		},
		{
			Name:  "getonlineuserinfo",
			Usage: "getonlineuserinfo <room> <username>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdGetOnlineUserInfo(ctx, args)
			},
		},
		{
			Name:  "getaccounts",
			Usage: "getaccounts <room>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdGetAccounts(ctx, args)
			},
		},
		{
			Name:  "createroom",
			Usage: "createroom <room>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdCreateRoom(ctx, args)
			},
		},
		{
			Name:  "deleteroom",
			Usage: "deleteroom <room>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdDeleteRoom(ctx, args)
			},
		},
		{
			Name:  "createaccount",
			Usage: "createaccount <room> <username> [password]",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdCreateAccount(ctx, args)
			},
		},
		{
			Name:  "deleteaccount",
			Usage: "deleteaccount <room> <username>",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdDeleteAccount(ctx, args)
			},
		},
		{
			Name:  "updateaccountpassword",
			Usage: "updateaccountpassword <room> <username> [password]",
			Handler: func(ctx context.Context, cli *Cli, args []string) error {
				return cli.cmdUpdateAccountPassword(ctx, args)
			},
		},
	}
	return cli
}

func (c *Cli) mkCtx() context.Context {
	ctx, callInfo := connect.NewClientContext(context.Background())
	for header, vals := range c.headers {
		if len(vals) == 0 {
			continue
		}

		callInfo.RequestHeader().Set(header, vals[0])
	}

	return ctx
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
			return cmd.Handler(c.mkCtx(), c, parts[1:])
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %q. Type \"help\" to see a list of commands.\n", name)

	return nil
}

func (c *Cli) cmdHelp(_ context.Context, args []string) error {
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

func (c *Cli) cmdExit(_ context.Context, _ []string) error {
	return errStop
}

func (c *Cli) cmdGetRooms(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 0, 0, "getrooms"); err != nil {
		return err
	}

	resp, err := c.client.GetRooms(ctx, &v1.GetRoomsRequest{})
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

func (c *Cli) cmdGetRoomInfo(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 1, 1, "getroominfo <room>"); err != nil {
		return err
	}

	resp, err := c.client.GetRoomInfo(ctx, &v1.GetRoomInfoRequest{
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

func (c *Cli) cmdGetOnlineUsers(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 1, 1, "getonlineusers <room>"); err != nil {
		return err
	}

	stream, err := c.client.GetOnlineUsers(ctx, &v1.GetOnlineUsersRequest{
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

func (c *Cli) cmdGetOnlineUserInfo(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 2, 2, "getonlineuserinfo <room> <username>"); err != nil {
		return err
	}

	resp, err := c.client.GetOnlineUserInfo(ctx, &v1.GetOnlineUserInfoRequest{
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

func (c *Cli) cmdGetAccounts(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 1, 1, "getaccounts <room>"); err != nil {
		return err
	}

	resp, err := c.client.GetAccounts(ctx, &v1.GetAccountsRequest{
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

func (c *Cli) cmdCreateRoom(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 1, 1, "createroom <room>"); err != nil {
		return err
	}

	resp, err := c.client.CreateRoom(ctx, &v1.CreateRoomRequest{
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

func (c *Cli) cmdDeleteRoom(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 1, 1, "deleteroom <room>"); err != nil {
		return err
	}

	_, err := c.client.DeleteRoom(ctx, &v1.DeleteRoomRequest{
		Name: args[0],
	})
	if err != nil {
		return err
	}

	fmt.Printf("Deleted room %q.\n", args[0])
	return nil
}

func (c *Cli) cmdCreateAccount(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 2, 3, "createaccount <room> <username> [password]"); err != nil {
		return err
	}

	pass := ""
	if len(args) == 3 {
		pass = args[2]
	}

	resp, err := c.client.CreateAccount(ctx, &v1.CreateAccountRequest{
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

func (c *Cli) cmdDeleteAccount(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 2, 2, "deleteaccount <room> <username>"); err != nil {
		return err
	}

	_, err := c.client.DeleteAccount(ctx, &v1.DeleteAccountRequest{
		Room:     args[0],
		Username: args[1],
	})
	if err != nil {
		return err
	}

	fmt.Printf("Deleted account %q in room %q.\n", args[1], args[0])
	return nil
}

func (c *Cli) cmdUpdateAccountPassword(ctx context.Context, args []string) error {
	if err := validateArgCount(args, 2, 3, "updateaccountpassword <room> <username> [password]"); err != nil {
		return err
	}

	pass := ""
	if len(args) == 3 {
		pass = args[2]
	}

	resp, err := c.client.UpdateAccountPassword(ctx, &v1.UpdateAccountPasswordRequest{
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

const defaultWelcomeMsg = "Welcome to the FriendNet server RPC CLI."

// Run runs the CLI.
// It returns when the client presses CTRL+D.
func (c *Cli) Run() {
	var msg string
	if c.welcomeMsg == "" {
		msg = defaultWelcomeMsg
	} else {
		msg = c.welcomeMsg
	}

	println(msg + "\nType \"help\" for a list of commands.")
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
			if errors.Is(doErr, errStop) {
				break
			}

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
