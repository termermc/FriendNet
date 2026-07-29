# Search Filtering

Sometimes, for legal or ethical reasons, you may want to filter out certain search results serverside. It is possible
to filter out search results for files whose filenames or paths contain blacklisted keywords.

> NOTE: Search filtering only works for serverwide search; it cannot filter direct user searches because those are
> peer-to-peer.

Filters can be applied globally across the entire server, or on a per-room basis, or a mix of both.

## Filter Match Modes

Keywords can be matches using several modes:
 - `Substring`
   
   > Any instance of the word is matched, even if within another word.
   > For example, `cat` will match to `cats`, `concatenate`, `category`, etc.

 - `Whole Word`

   > Only whole words will be matched.
   > For example, `stack` will match to `stack`, but not `stackable`, `stacks`, etc.

 - `Regular Expression`

   > Terms matching the specified [regular expression](https://github.com/google/re2/wiki/Syntax) will be matched.
   > For example, `c[a-z]t` will match to `cat`, `cut`, `cot`, etc.
   > 
   > Note that regular expressions are case-sensitive.

## Adding From the Admin UI

If you have the [admin UI](setup/management.md) set up, you can add keywords from there.

TODO Screenshot and directions

## Adding From the RPC or CLI

If you are running the server in a terminal or have access to it via the RPC client, you can use that to add keywords.

Type `help` to see all the available blacklist commands.

Currently, these are the commands that exist for managing blacklisted keywords:

 - addglobalblacklistpolicies <substring|whole|regex> <keywords... (one or more)>
 - addroomblacklistpolicies <room> <substring|whole|regex> <keywords>
 - removeglobalblacklistpolicies removeglobalblacklistpolicies <keywords>
 - removeroomblacklistpolicies <room> <keywords>
 - listblacklistpolicies [room]
