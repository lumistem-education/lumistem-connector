{
  description = "Nix flake for auth-services in LumiSTEM projects";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/5adc50831c259f87faeb09ddf930d43597197473";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in {
      devShells = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [ 
              go
              gopls
            ];
            shellHook = "export CGO_ENABLED=0";
          };
        });
    };
}
