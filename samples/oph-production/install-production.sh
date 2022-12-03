#!/usr/bin/env sh
set -eu

# Open Payment Host production setup using `docker-compose`.
# See https://github.com/abishekmuthian/open-payment-host for detailed installation steps.

alias up="cd .."

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
	mkdir certs
	mkdir public
	cd public
	mkdir assets
	cd  assets 
	mkdir icons
	mkdir images
	cd images
	mkdir app
	mkdir products
	cd -
	mkdir scripts
	mkdir styles
	up
	up
}

setup_db(){
	cd db
	curl -o Create-Tables.sql https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/db/Create-Tables.sql
    cd -
}

setup_containers() {
	curl -o docker-compose.yml https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-production/docker-compose.yml
	docker-compose up -d
}

show_output(){
	echo -e "\nOpen Payment Host is now up and running. Stop the container, Set the required production configuration and re-run the container.\n"
}


check_dependencies
setup_folders
setup_db
setup_containers
show_output