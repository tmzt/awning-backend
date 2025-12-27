
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
	rsync -avvz -e "ssh -p 2022" ../dist/* sanctum-admin@sanctumai.app:/srv/apps/sanctum/app/public/

logs:
	docker-compose -f docker-compose.beta.yml logs -f
