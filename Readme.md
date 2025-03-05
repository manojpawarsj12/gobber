# Gobber

Gobber is a package manager for Go, designed to simplify the process of managing dependencies for your Go projects.

## Installation

### Using `go install`

To install Gobber directly using `go install`, run the following command:

```sh
go install github.com/manojpawarsj12/gobber/cmd/gobber@latest
```

To install Gobber, clone the repository and build the project:

```sh
git clone https://github.com/manojpawarsj12/gobber.git
cd gobber
go build -o gobber cmd/main.go
```

## Usage

### Install Packages

To install packages, use the following command:

```sh
gobber install <package-names>
```

You can also use the alias `i`:

```sh
gobber i <package-names>
```

If no package names are provided, Gobber will read from `package.json` and install the dependencies listed there.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
