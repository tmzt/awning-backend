
build-api:
	go build -o ./bin/awning-api .

run:
	go run .

build-up:
	docker-compose -f docker-compose.beta.yml up --build -d

up:
	docker-compose -f docker-compose.beta.yml up -d

down:
	docker-compose -f docker-compose.beta.yml down

restart:
	docker-compose -f docker-compose.beta.yml restart

build:
	npm run build

deploy:
	rsync -avvz -e "ssh -p 2022" ../dist/* core-admin@awning.app:/srv/apps/awning/app/static/public/

logs:
	docker-compose -f docker-compose.beta.yml logs -f

deploy-up:
	docker-compose -f docker-compose.beta-deploy.yml up -d

deploy-down:
	docker-compose -f docker-compose.beta-deploy.yml down

deploy-restart:
	docker-compose -f docker-compose.beta-deploy.yml restart

deploy-logs:
	docker-compose -f docker-compose.beta-deploy.yml logs -f

deploy-configs:
	rsync -avvz -L -e "ssh -p 2022" ./.config/config.json core-admin@awning.app:/srv/apps/awning_backend/config/
	rsync -avvz -L -e "ssh -p 2022" ./.config/prompts core-admin@awning.app:/srv/apps/awning_backend/config/
	rsync -avvz -L -e "ssh -p 2022" ./.private/service_credentials.json core-admin@awning.app:/srv/apps/awning_backend/creds/
