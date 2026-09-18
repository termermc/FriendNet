{
  description = "FriendNet build and development environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;

      # The application version lives in Go source, where it is also what the client
      # and server report at runtime (see server/rpc.go and client/rpc.go). Read it
      # from there so this flake cannot drift out of sync with the built binaries.
      version =
        let
          inherit (nixpkgs) lib;
          matches = map
            (builtins.match ''[[:space:]]*Version:[[:space:]]*"([^"]+)",[[:space:]]*'')
            (lib.splitString "\n" (builtins.readFile ./updater/update.go));
        in
        builtins.head (lib.findFirst (m: m != null)
          (throw "could not extract CurrentUpdate.Version from updater/update.go")
          matches);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          inherit (pkgs) lib buildGo127Module buildNpmPackage importNpmLock nodejs_24 makeWrapper stdenv xdg-utils;
          client = let
            webui = buildNpmPackage {
              pname = "friendnet-webui";
              inherit version;
              src = lib.cleanSourceWith {
                src = ./webui;
                filter = path: type:
                  !(builtins.elem (baseNameOf path) [ "node_modules" "dist" ])
                  && lib.cleanSourceFilter path type;
              };
              nodejs = nodejs_24;
              # Fetch each dependency using the integrity hash already recorded in
              # package-lock.json, rather than one aggregate hash that would have to be
              # updated by hand whenever a dependency changes.
              npmDeps = importNpmLock { npmRoot = ./webui; };
              npmConfigHook = importNpmLock.npmConfigHook;
              installPhase = ''
                runHook preInstall
                mkdir -p "$out"
                cp -r dist/. "$out/"
                runHook postInstall
              '';
            };
          in
          buildGo127Module {
            pname = "friendnet-client";
            inherit version;
            src = lib.fileset.toSource {
              root = ./.;
              fileset = lib.fileset.unions ((map
                (dir: lib.fileset.fileFilter
                  (file: file.hasExt "go" || file.name == "go.mod" || file.name == "go.sum")
                  dir)
                [ ./browser ./client ./common ./mkcert ./protocol ./stun ./updater ./upnp ]
              ) ++ [ ./webui/webui.go ./webui/go.mod ]);
            };
            modRoot = "client";
            # Tie the module set to go.sum so that changing dependencies without
            # updating vendorHash fails loudly, instead of silently reusing the
            # previously realised (and now stale) module set.
            goSum = ./client/go.sum;
            env = {
              CGO_ENABLED = "0";
              GOWORK = "off";
            };
            vendorHash = "sha256-czq0yOK3xlTZDMcEXgp428ZW9fSTwSAF5jn07MlJ2+Q=";
            # Keep local modules outside the dependency hash so source/UI edits do not
            # require rehashing dependencies.
            proxyVendor = true;
            subPackages = [ "cmd/client" ];

            postConfigure = ''
              mkdir -p ../webui/dist
              cp -r ${webui}/. ../webui/dist/
            '';

            nativeBuildInputs = lib.optionals stdenv.hostPlatform.isLinux [ makeWrapper ];
            postInstall = ''
              mv "$out/bin/client" "$out/bin/friendnet-client"
            '' + lib.optionalString stdenv.hostPlatform.isLinux ''
              wrapProgram "$out/bin/friendnet-client" \
                --suffix PATH : ${lib.makeBinPath [ xdg-utils ]}
            '';

            meta = {
              description = "Peer-to-peer file sharing for friends";
              homepage = "https://friendnet.org";
              license = lib.licenses.gpl3Only;
              mainProgram = "friendnet-client";
              platforms = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
            };
          };
          server = let
            adminui = buildNpmPackage {
              pname = "friendnet-adminui";
              inherit version;
              src = lib.cleanSourceWith {
                src = ./adminui;
                filter = path: type:
                  !(builtins.elem (baseNameOf path) [ "node_modules" "dist" ])
                  && lib.cleanSourceFilter path type;
              };
              nodejs = nodejs_24;
              # Derived from package-lock.json, as for the client web UI.
              npmDeps = importNpmLock { npmRoot = ./adminui; };
              npmConfigHook = importNpmLock.npmConfigHook;
              installPhase = ''
                runHook preInstall
                mkdir -p "$out"
                cp -r dist/. "$out/"
                runHook postInstall
              '';
            };
          in
          buildGo127Module {
            pname = "friendnet-server";
            inherit version;
            src = lib.fileset.toSource {
              root = ./.;
              fileset = lib.fileset.unions ((map
                (dir: lib.fileset.fileFilter
                  (file: file.hasExt "go" || file.name == "go.mod" || file.name == "go.sum")
                  dir)
                [ ./ahocorasick ./common ./protocol ./rpcclient ./server ./stun ./updater ]
              ) ++ [ ./adminui/adminui.go ./adminui/go.mod ]);
            };
            modRoot = "server";
            goSum = ./server/go.sum;
            env = {
              CGO_ENABLED = "0";
              GOWORK = "off";
            };
            vendorHash = "sha256-U8ZIGIrGrr0Tj7jYvz25bPSkyOd5RjwX8meQdDTxzLo=";
            # Keep local modules outside the dependency hash, as for the client package.
            proxyVendor = true;
            subPackages = [ "cmd/server" ];
            ldflags = [ "-s" "-w" ];

            postConfigure = ''
              mkdir -p ../adminui/dist
              cp -r ${adminui}/. ../adminui/dist/
            '';

            postInstall = ''
              mv "$out/bin/server" "$out/bin/friendnet-server"
            '';

            meta = {
              description = "Server for FriendNet peer-to-peer file sharing";
              homepage = "https://friendnet.org";
              license = lib.licenses.agpl3Only;
              mainProgram = "friendnet-server";
              platforms = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
            };
          };
          rpcclient = buildGo127Module {
            pname = "friendnet-rpcclient";
            inherit version;
            src = lib.fileset.toSource {
              root = ./.;
              fileset = lib.fileset.unions (map
                (dir: lib.fileset.fileFilter
                  (file: file.hasExt "go" || file.name == "go.mod" || file.name == "go.sum")
                  dir)
                [ ./common ./protocol ./rpcclient ]);
            };
            modRoot = "rpcclient";
            goSum = ./rpcclient/go.sum;
            env = {
              CGO_ENABLED = "0";
              GOWORK = "off";
            };
            vendorHash = "sha256-14ZcKDU/Hv8WCrVLqioRX98E0ZlmFpuydbILDdikKr4=";
            proxyVendor = true;
            subPackages = [ "cmd/cli" ];
            ldflags = [ "-s" "-w" ];

            postInstall = ''
              mv "$out/bin/cli" "$out/bin/friendnet-rpcclient"
            '';

            meta = {
              description = "Command-line administration client for FriendNet servers";
              homepage = "https://friendnet.org";
              license = lib.licenses.mit;
              mainProgram = "friendnet-rpcclient";
              platforms = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
            };
          };
        in
        {
          inherit client server rpcclient;
          default = client;
        });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go_1_27
              pkgs.nodejs_24
              pkgs.git
            ] ++ pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [
              pkgs.xdg-utils
            ];

            CGO_ENABLED = "0";
            GOTOOLCHAIN = "local";
          };
        });
    };
}
