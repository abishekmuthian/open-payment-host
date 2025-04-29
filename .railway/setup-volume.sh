#!/usr/bin/env sh
set -eu

alias up="cd .."

setup_folders(){
    cd /data
	echo -e "\nCreating folders if necessary and mounting them.\n"
	if [ ! -d "/data/secrets" ]; then
		mkdir secrets
	fi
	sudo mount -o bind /data/secrets /app/secrets
	if [ ! -d "/data/db" ]; then
		mkdir db
	fi
	sudo mount -o bind /data/db /app/db
	if [ ! -d "/data/log" ]; then
		mkdir log
	fi
	sudo mount -o bind /data/log /app/log
    if [ ! -d "/data/certs" ]; then
		mkdir certs
	fi
	sudo mount -o bind /data/certs /app/certs
    if [ ! -d "/data/public" ]; then
		mkdir public
		cd public
		if [ ! -d "/data/public/assets" ]; then
			mkdir assets
		fi
		cd  assets
		if [ ! -d "/data/public/assets/icons" ]; then
			mkdir icons
		fi
		if [ ! -d "/data/public/assets/images" ]; then
			mkdir images
		fi
		cd images
		if [ ! -d "/data/public/assets/images/app" ]; then
			mkdir app
		fi
		if [ ! -d "/data/public/assets/images/products" ]; then
			mkdir products
		fi		
		cd -
		if [ ! -d "/data/public/assets/scripts" ]; then
			mkdir scripts
		fi
		if [ ! -d "/data/public/assets/styles" ]; then
			mkdir styles
		fi						
		up
		up
	fi
	sudo mount -o bind /data/public /app/public
	echo -e "\nSetting up permissions.\n"
	sudo chown -R default:default /data				
    echo -e "\nFolders setup successfully.\n"
}

setup_db(){
	cd db
	curl -o Create-Tables.sql https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/db/Create-Tables.sql
    cd -
	echo -e "\nDB setup successfully.\n"
}

navigate_to_app(){
	cd /app/
    echo -e "\nNavigated to the app folder.\n"
}

run_app(){
    /app/exec/open-payment-host
    echo -e "\nOpen Payment Host Started.\n"
}

setup_folders
setup_db
navigate_to_app
run_app