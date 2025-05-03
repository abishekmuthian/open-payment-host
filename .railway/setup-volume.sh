#!/usr/bin/env sh
set -eu

alias up="cd .."

setup_folders(){
    cd /home/default/build/data
	echo -e "\nCreating folders if necessary and mounting them.\n"
	if [ ! -d "/home/default/build/data/secrets" ]; then
		mkdir secrets
	fi
	if [ ! -d "/home/default/build/data/db" ]; then
		mkdir db
	fi
	if [ ! -d "/home/default/build/data/log" ]; then
		mkdir log
	fi
    if [ ! -d "/home/default/build/data/certs" ]; then
		mkdir certs
	fi
    if [ ! -d "/home/default/build/data/public" ]; then
	    mkdir public
		cp -a /home/default/build/setup/public/* public/
	fi
	echo -e "\nSetting up permissions.\n"
	sudo chown -R default:default /home/default/build/data				
    echo -e "\nFolders setup successfully.\n"
}

setup_db(){
	cd db
	curl -o Create-Tables.sql https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/db/Create-Tables.sql
    cd -
	echo -e "\nDB setup successfully.\n"
}

navigate_to_app(){
	cd /home/default/build/
	cd /home/default/build/
    echo -e "\nNavigated to the app folder.\n"
}

run_app(){
    /home/default/build/exec/open-payment-host
    /home/default/build/exec/open-payment-host
    echo -e "\nOpen Payment Host Started.\n"
}

setup_folders
setup_db
navigate_to_app
run_app