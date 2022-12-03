#!/usr/bin/env sh
set -eu

# Open Payment Host demo setup using `docker-compose`.
# See https://github.com/abishekmuthian/open-payment-host for detailed installation steps.

check_dependencies() {
	if ! command -v curl > /dev/null; then
		echo "curl is not installed."
		exit 1
	fi

	if ! command -v docker > /dev/null; then
		echo "docker is not installed."
		exit 1
	fi

	if ! command -v docker-compose > /dev/null; then
		echo "docker-compose is not installed."
		exit 1
	fi
}

setup_folders(){
	mkdir secrets
	mkdir db
	mkdir log
	mkdir --p public/assets/images/products
}

setup_db(){
	cd db
	curl -o Create-Tables.sql https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/db/Create-Tables.sql
	cd -
}

setup_containers() {
	curl -o docker-compose.yml https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-demo/docker-compose.yml
	docker-compose up -d
}

show_output(){
	echo -e "\nOpen Payment Host is now up and running. Visit http://localhost:3000 in your browser.\n"
}


check_dependencies
setup_folders
setup_db
setup_containers
show_output