# include .envrc

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## run/api: run the cmd/api application
.PHONY: run/api
run/api:
	@go run ./cmd/api \
		-db-dsn=${APPA_DB_DSN} \
		-smtp-host=${SMTP_HOST} \
		-smtp-port=${SMTP_PORT} \
		-smtp-username=${SMTP_USERNAME} \
		-smtp-password=${SMTP_PASSWORD} \
		-smtp-sender=${SMTP_SENDER}

## db/psql: connect to the database using psql
.PHONY: db/psql
db/psql:
	@psql ${APPA_DB_DSN}

## db/migrations/new name=$1: create a new databse migration
.PHONY: db/migrations/new
db/migrations/new:
	@echo 'Creating migration files for ${name}...'
	@migrate create -seq -ext=.sql -dir=./migrations ${name}

## db/migrations/up: apply all up database migrations
.PHONY: db/migrations/up
db/migrations/up: confirm
	@echo 'Running up migrations...'
	@migrate -path ./migrations -database ${APPA_DB_DSN} up

# ==================================================================================== #
# VAGRANT / ANSIBLE (infrastructure)
# ==================================================================================== #

VAGRANT_DIR = deploy/ansible/dev
VENV_BIN    = $(CURDIR)/deploy/ansible/.venv/bin

## vagrant/up: create and start the dev VM (activate venv first, then vagrant)
.PHONY: vagrant/up
vagrant/up:
	cd $(VAGRANT_DIR) && PATH="$(VENV_BIN):$$PATH" vagrant up

## vagrant/destroy: destroy the dev VM
.PHONY: vagrant/destroy
vagrant/destroy:
	cd $(VAGRANT_DIR) && PATH="$(VENV_BIN):$$PATH" vagrant destroy -f

## vagrant/reload: reboot and re-provision the dev VM
.PHONY: vagrant/reload
vagrant/reload:
	cd $(VAGRANT_DIR) && PATH="$(VENV_BIN):$$PATH" vagrant reload --provision

## vagrant/ssh: SSH into the dev VM
.PHONY: vagrant/ssh
vagrant/ssh:
	cd $(VAGRANT_DIR) && vagrant ssh

## vagrant/status: show VM status
.PHONY: vagrant/status
vagrant/status:
	cd $(VAGRANT_DIR) && vagrant status

## ansible/lint: run ansible-lint on all Ansible files
.PHONY: ansible/lint
ansible/lint:
	PATH="$(VENV_BIN):$$PATH" ansible-lint $(ARGS)

ANSIBLE_DIR = deploy/ansible
ROLES       = kernel_hardening access_control ssh_hardening firewall audit

## ansible/molecule/role: run molecule on a role (ROLE=<name>, CMD=<command>)
.PHONY: ansible/molecule/role
ansible/molecule/role:
	@if [ -z "$(ROLE)" ]; then echo "Usage: make ansible/molecule/role ROLE=<name> [CMD=test]"; exit 1; fi
	@cd $(ANSIBLE_DIR)/roles/$(ROLE) && PATH="$(VENV_BIN):$$PATH" molecule $(CMD)

## ansible/molecule/playbook: run molecule on the playbook scenario (CMD=<command>)
.PHONY: ansible/molecule/playbook
ansible/molecule/playbook:
	@cd $(ANSIBLE_DIR)/playbooks/security-hardening && PATH="$(VENV_BIN):$$PATH" molecule $(CMD)

## ansible/molecule/test/all: run full molecule test on all roles and playbook
.PHONY: ansible/molecule/test/all
ansible/molecule/test/all:
	@for role in $(ROLES); do \
		echo "=== Testing role: $$role ==="; \
		cd $(ANSIBLE_DIR)/roles/$$role && PATH="$(VENV_BIN):$$PATH" molecule test || exit 1; \
	done
	@echo "=== Testing playbook: security-hardening ==="; \
	cd $(ANSIBLE_DIR)/playbooks/security-hardening && PATH="$(VENV_BIN):$$PATH" molecule test

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: tidy module dependencies and format all .go files
.PHONY: tidy
tidy:
	@echo 'Tidy module dependencies...'
	go mod tidy
	@echo 'Formatting .go files...'
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit:
	@echo 'Checking module dependencies...'
	go mod tidy -diff
	go mod verify
	@echo 'Vetting code...'
	go vet ./...
	go tool staticcheck ./...
	@echo 'Running tests...'
	go test -race -vet=off ./...

# ==================================================================================== #
# BUILD
# ==================================================================================== #

## build/api: build the cmd/api application
.PHONY: build/api
build/api:
	@echo 'Building cmd/api...'
	go build -ldflags='-s' -o=./bin/api ./cmd/api
	GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/api ./cmd/api
