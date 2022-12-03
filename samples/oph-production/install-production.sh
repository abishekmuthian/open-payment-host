#!/usr/bin/env sh
set -eu

# Open Payment Host production setup using `docker-compose`.
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
	mkdir certs
	mkdir --p public/assets/icons
	mkdir --p public/assets/images/app
	mkdir --p public/assets/images/products
}

setup_env(){
	curl -o fragmenta.env https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-production/fragmenta.env
}

setup_db(){
	cd db
	curl -o Create-Tables.sql https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/db/Create-Tables.sql
    cd -
}

setup_images(){
	curl -o favicon.ico https://raw.githubusercontent.com/abishekmuthian/open-payment-host/blob/main/public/favicon.ico
    cd public/assets/images/app
	curl -o favicon.ico https://raw.githubusercontent.com/abishekmuthian/open-payment-host/blob/main/public/assets/images/app/oph_featured_image.png
    cd -
}

setup_containers() {
	curl -o docker-compose.yml https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-production/docker-compose.yml
	docker-compose --env-file fragmenta.env up -d
}

show_output(){
	echo -e "\nOpen Payment Host is now up and running. Visit http://localhost:443 in your browser.\n"
}


check_dependencies
setup_folders
setup_env
setup_db
setup_images
setup_containers
show_output