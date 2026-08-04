package rpcclient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"friendnet.org/common"
	v1 "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"github.com/chzyer/readline"
)

var matchModeToName = map[v1.BlacklistMatchMode]string{
	v1.BlacklistMatchMode_BLACKLIST_MATCH_MODE_SUBSTRING: "substring",
	v1.BlacklistMatchMode_BLACKLIST_MATCH_MODE_WHOLE:     "whole",
	v1.BlacklistMatchMode_BLACKLIST_MATCH_MODE_REGEX:     "regex",
}
var nameToMatchMode = func() map[string]v1.BlacklistMatchMode {
	res := make(map[string]v1.BlacklistMatchMode, len(matchModeToName))
	for mode, name := range matchModeToName {
		res[name] = mode
	}
	return res
}()
var matchModeNames = func() []string {
	names := make([]string, 0, len(nameToMatchMode))
	for _, name := range matchModeToName {
		names = append(names, name)
	}
	return names
}()

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
	Handler func(ctx context.Context, cmd Cmd, args []string) error
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
			Name:    "help",
			Usage:   "[command]",
			Handler: cli.cmdHelp,
		},
		{
			Name:    "exit",
			Usage:   "exit",
			Handler: cli.cmdExit,
		},
		{
			Name:    "getserverinfo",
			Usage:   "",
			Handler: cli.cmdGetServerInfo,
		},
		{
			Name:    "getrooms",
			Usage:   "",
			Handler: cli.cmdGetRooms,
		},
		{
			Name:    "getroominfo",
			Usage:   "<room>",
			Handler: cli.cmdGetRoomInfo,
		},
		{
			Name:    "getonlineusers",
			Usage:   "<room>",
			Handler: cli.cmdGetOnlineUsers,
		},
		{
			Name:    "getonlineuserinfo",
			Usage:   "<room> <username>",
			Handler: cli.cmdGetOnlineUserInfo,
		},
		{
			Name:    "getaccounts",
			Usage:   "<room>",
			Handler: cli.cmdGetAccounts,
		},
		{
			Name:    "createroom",
			Usage:   "<room>",
			Handler: cli.cmdCreateRoom,
		},
		{
			Name:    "deleteroom",
			Usage:   "<room>",
			Handler: cli.cmdDeleteRoom,
		},
		{
			Name:    "createaccount",
			Usage:   "<room> <username> [password]",
			Handler: cli.cmdCreateAccount,
		},
		{
			Name:    "deleteaccount",
			Usage:   "<room> <username>",
			Handler: cli.cmdDeleteAccount,
		},
		{
			Name:    "updateaccountpassword",
			Usage:   "<room> <username> [password]",
			Handler: cli.cmdUpdateAccountPassword,
		},
		{
			Name:    "addglobalblacklistpolicies",
			Usage:   fmt.Sprintf("<%s> <keywords... (one or more)>", strings.Join(matchModeNames, "|")),
			Handler: cli.cmdAddGlobalBlacklistPolicies,
		},
		{
			Name:    "addroomblacklistpolicies",
			Usage:   fmt.Sprintf("<room> <%s> <keywords>", strings.Join(matchModeNames, "|")),
			Handler: cli.cmdAddRoomBlacklistPolicies,
		},
		{
			Name:    "removeglobalblacklistpolicies",
			Usage:   "removeglobalblacklistpolicies <keywords>",
			Handler: cli.cmdRemoveGlobalBlacklistPolicies,
		},
		{
			Name:    "removeroomblacklistpolicies",
			Usage:   "<room> <keywords>",
			Handler: cli.cmdRemoveRoomBlacklistPolicies,
		},
		{
			Name:    "getblacklistpolicies",
			Usage:   "[room]",
			Handler: cli.cmdGetBlacklistPolicies,
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
			return cmd.Handler(c.mkCtx(), cmd, parts[1:])
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %q. Type \"help\" to see a list of commands.\n", name)

	return nil
}

func (c *Cli) cmdHelp(_ context.Context, _ Cmd, args []string) error {
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

func (c *Cli) cmdExit(_ context.Context, _ Cmd, _ []string) error {
	return errStop
}

func (c *Cli) cmdGetServerInfo(ctx context.Context, _ Cmd, _ []string) error {
	resp, err := c.client.GetServerInfo(ctx, &v1.GetServerInfoRequest{})
	if err != nil {
		return err
	}

	fmt.Printf("Server version: %s\n", resp.GetVersion())
	fmt.Printf("RPC requires bearer token authentication: %t\n", resp.GetRpc().GetRequiresBearerToken())
	allowedMethods := resp.GetRpc().GetAllowedMethods()
	if slices.Contains(allowedMethods, "*") {
		fmt.Printf("Allowed methods for this RPC interface: all\n")
	} else {
		fmt.Printf("Allowed methods for this RPC interface:\n")
		for _, method := range resp.GetRpc().GetAllowedMethods() {
			fmt.Printf("  %s\n", method)
		}
	}

	return nil
}

func (c *Cli) cmdGetRooms(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 0, 0, cmd); err != nil {
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

func (c *Cli) cmdGetRoomInfo(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 1, cmd); err != nil {
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

func (c *Cli) cmdGetOnlineUsers(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 1, cmd); err != nil {
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

func (c *Cli) cmdGetOnlineUserInfo(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 2, 2, cmd); err != nil {
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

func (c *Cli) cmdGetAccounts(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 1, cmd); err != nil {
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

func (c *Cli) cmdCreateRoom(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 1, cmd); err != nil {
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

func (c *Cli) cmdDeleteRoom(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 1, cmd); err != nil {
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

func (c *Cli) cmdCreateAccount(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 2, 3, cmd); err != nil {
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

func (c *Cli) cmdDeleteAccount(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 2, 2, cmd); err != nil {
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

func (c *Cli) cmdUpdateAccountPassword(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 2, 3, cmd); err != nil {
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

func (c *Cli) addBlacklistPolicies(ctx context.Context, room string, modeName string, keywords []string) error {
	mode, ok := nameToMatchMode[modeName]
	if !ok {
		return fmt.Errorf("unknown match mode %q", modeName)
	}

	policiesEmpty := make([]v1.BlacklistPolicy, len(keywords))
	policies := make([]*v1.BlacklistPolicy, len(keywords))
	for i, keyword := range keywords {
		policy := &policiesEmpty[i]
		policy.Keyword = keyword
		policy.Mode = mode
		policies[i] = policy
	}

	_, err := c.client.AddBlacklistPolicies(ctx, &v1.AddBlacklistPoliciesRequest{
		Room:     common.StrOrNil(room),
		Policies: policies,
	})
	return err
}

func (c *Cli) cmdAddGlobalBlacklistPolicies(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 2, math.MaxInt64, cmd); err != nil {
		return err
	}

	return c.addBlacklistPolicies(ctx, "", args[0], args[1:])
}

func (c *Cli) cmdRemoveGlobalBlacklistPolicies(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, math.MaxInt64, cmd); err != nil {
		return err
	}

	var room *string
	if len(args) == 2 {
		room = &args[1]
	}

	_, err := c.client.RemoveBlacklistPolicies(ctx, &v1.RemoveBlacklistPoliciesRequest{
		Room:     room,
		Policies: args,
	})

	if err != nil {
		return err
	}

	return nil
}

func (c *Cli) cmdAddRoomBlacklistPolicies(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 3, math.MaxInt64, cmd); err != nil {
		return err
	}

	return c.addBlacklistPolicies(ctx, args[0], args[1], args[2:])
}

func (c *Cli) cmdRemoveRoomBlacklistPolicies(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 1, 2, cmd); err != nil {
		return err
	}

	var room *string
	if len(args) == 2 {
		room = &args[1]
	}

	_, err := c.client.RemoveBlacklistPolicies(ctx, &v1.RemoveBlacklistPoliciesRequest{
		Room:     room,
		Policies: args,
	})

	if err != nil {
		return err
	}

	return nil
}

func (c *Cli) cmdGetBlacklistPolicies(ctx context.Context, cmd Cmd, args []string) error {
	if err := validateArgCount(args, 0, 1, cmd); err != nil {
		return err
	}

	var room *string
	if len(args) == 1 {
		room = &args[0]
	}

	resp, err := c.client.GetBlacklistPolicies(ctx, &v1.GetBlacklistPoliciesRequest{
		Room: room,
	})
	if err != nil {
		return err
	}

	for _, policy := range resp.Policies {
		name, has := matchModeToName[policy.Mode]
		if !has {
			name = "unknown"
		}

		println(name + " " + policy.Keyword)
	}

	return nil
}

func cmdUsageMsg(cmd Cmd) string {
	if cmd.Usage == "" {
		return "usage: " + cmd.Name
	}

	return "usage: " + cmd.Name + " " + cmd.Usage
}

func validateArgCount(args []string, min int, max int, cmd Cmd) error {
	if len(args) < min || (max >= 0 && len(args) > max) {
		return errors.New(cmdUsageMsg(cmd))
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
