# Changelog

All notable changes to this project will be documented in this file.

## 1.0.0 - 2015-05-18

* Refactoring error responses. Was `res, apiError, err :=`, now is just `res, err :=`
* Adding integration testing keys (`SANDBOX_SUCCESS`, `SANDBOX_ERROR`)

## 0.0.2 - 2014-11-22

* Fixing issue where send_at, async, and ip_pool payload attributes were set on the `message` attribute. Needs to be at the root level.

## 0.0.1 - 2014-10-27

* Intitial version!
