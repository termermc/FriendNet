{
  description = "FriendNet build and development environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          inherit (pkgs) lib buildGo127Module buildNpmPackage nodejs_24 makeWrapper stdenv xdg-utils;
          client = let
            webui = buildNpmPackage {
              pname = "friendnet-webui";
              version = "unstable";
              src = lib.cleanSourceWith {
                src = ./webui;
                filter = path: type:
                  !(builtins.elem (baseNameOf path) [ "node_modules" "dist" ])
                  && lib.cleanSourceFilter path type;
              };
              nodejs = nodejs_24;
              npmDepsHash = "sha256-1e/UzxtXcmbDTXl+0xDrbj5LylboflmGcyovXo/ahsk=";
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
            version = "unstable";
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
              version = "unstable";
              src = lib.cleanSourceWith {
                src = ./adminui;
                filter = path: type:
                  !(builtins.elem (baseNameOf path) [ "node_modules" "dist" ])
                  && lib.cleanSourceFilter path type;
              };
              nodejs = nodejs_24;
              npmDepsHash = "sha256-hrzheA7HOX1aYQK7YNRi4Vr08rkRInb3wYtDRejb1/w=";
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
            version = "unstable";
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
            version = "unstable";
            src = lib.fileset.toSource {
              root = ./.;
              fileset = lib.fileset.unions (map
                (dir: lib.fileset.fileFilter
                  (file: file.hasExt "go" || file.name == "go.mod" || file.name == "go.sum")
                  dir)
                [ ./common ./protocol ./rpcclient ]);
            };
            modRoot = "rpcclient";
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
