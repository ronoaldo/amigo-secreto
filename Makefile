# Aws Configuration
AWS=aws --profile=prod
ORGANIZATION=$(shell aws --profile prod sts get-caller-identity --query='Account')
FN=amigo-secreto

# Archtecture
GOARCH=amd64
LAMBDA_ARCH=$(if eq GOARCH "amd64", x86_64, amr64)

# Source/dest
SRC=$(shell find ./ -iname '*.go' -o -iname '*.html')
PROG=bootstrap
ZIP=amigo-secreto-lambda.zip


all: test build package

clean:
	rm -rf $(PROG) $(ZIP)

build: $(PROG)
$(PROG): $(SRC)
	GOOS=linux GOARCH=$(GOARCH) go build -tags lampda.norpc -o $(PROG) ./cmd/bootstrap

test:
	GO_FLAGS=-count=1 go test -v ./...

package: $(ZIP)
$(ZIP): $(PROG)
	[ -f $(ZIP) ] && rm -rf $(ZIP) || true
	zip $(ZIP) $(PROG)

deploy: $(ZIP)
	if $(AWS) lambda get-function --function-name $(FN) 2>/dev/null >/dev/null ; then \
		$(AWS) lambda update-function-code --function-name $(FN) --zip-file fileb://$(ZIP) ; \
	else \
		$(AWS) lambda create-function --function-name $(FN) \
			--runtime provided.al2023 --handler bootstrap \
			--architecture $(LAMBDA_ARCH) --zip-file fileb://$(ZIP) \
			--role arn:aws:iam::$(ORGANIZATION):role/lambda-ex ; \
		$(AWS) lambda add-permission --function-name $(FN) \
			--action lambda:InvokeFunctionUrl --principal "*" \
			--function-url-auth-type "NONE" \
			--statement-id url ; \
		$(AWS) lambda create-function-url-config --function-name $(FN) --auth-type NONE ; \
	fi
	echo "Function deployed at: "
	$(AWS) lambda get-function-url-config --function-name $(FN) --query FunctionUrl

undeploy:
	$(AWS) lambda delete-function-url-config --function-name $(FN) || echo "Not deployed"
	$(AWS) lambda delete-function --function-name $(FN) || echo "Not deployed"

setup-authentication:
	$(AWS) iam create-role --role-name lambda-ex \
		--assume-role-policy-document \
		'{"Version": "2012-10-17","Statement": [{ "Effect": "Allow", "Principal": {"Service": "lambda.amazonaws.com"}, "Action": "sts:AssumeRole"}]}'
	$(AWS) iam attach-role-policy --role-name lambda-ex --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole
