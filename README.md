# ash

[![Go](https://github.com/paolo-dellepiane/ash/actions/workflows/go.yml/badge.svg)](https://github.com/paolo-dellepiane/ash/actions/workflows/go.yml)

## Install

- Scoop Install

```
scoop install https://gist.githubusercontent.com/paolo-dellepiane/d299721ffd4077500e3d405d31fe9b9d/raw/ash.json
```

- Mac M1
  needs

```
go get -u golang.org/x/sys
```

## Release

```
git push -m "[skip ci] ..."
git tag 0.0.x
git push --tags
```

after action is completed and release is created:

```
update.ps1
```
