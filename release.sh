#!/bin/bash

# Usage: ./release.sh 1.1

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: ./release.sh <version>"
    echo "Example: ./release.sh 1.1"
    exit 1
fi

echo "Building binaries for v$VERSION..."
GOOS=linux GOARCH=amd64 go build -o gosip-linux
GOOS=darwin GOARCH=amd64 go build -o gosip-mac
GOOS=windows GOARCH=amd64 go build -o gosip.exe
echo "Binaries built!"

echo "Pushing to GitHub..."
git add .
git commit -m "release v$VERSION"
git tag v$VERSION
git push origin main
git push origin v$VERSION
echo "Pushed to GitHub!"

echo "Updating AUR..."
cd ~/gosip
sed -i "s/pkgver=.*/pkgver=$VERSION/" PKGBUILD
makepkg --printsrcinfo > .SRCINFO
git add PKGBUILD .SRCINFO
git commit -m "update to v$VERSION"
git push
echo "AUR updated!"

echo ""
echo "v$VERSION released!"
echo "Now go to GitHub and upload the binaries to the release:"
echo "https://github.com/Bearcry55/gosip/releases/new"
