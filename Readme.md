# Gobber

Gobber is a package manager for NPM, designed to simplify the process of managing dependencies for your Node.js projects.

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

To use Gobber, you can run the install command to install packages. For example, to install the latest version of the express package, you can use the following command:

```sh
./gobber install express@latest
```

## License

This project is licensed under the MIT License. See the LICENSE file for details.

