<!--
title: 'AWS Simple HTTP Endpoint example in Python'
description: 'This template demonstrates how to make a simple HTTP API with Python running on AWS Lambda and API Gateway using the Serverless'
-->

# Serverless Framework Python HTTP API on AWS

This template demonstrates how to make a simple HTTP API with Python running on AWS Lambda and API Gateway using the Serverless Framework.



## Usage

### Deployment
Following https://www.philschmid.de/serverless-bert-with-huggingface-aws-lambda-docker

Create ECR repository
aws_region=us-east-1
aws_account_id=<account_id>

> aws ecr create-repository --repository-name embedding-lambda --region $aws_region --profile dgraph
use repositoryUri
> docker tag python-lambda $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda


> aws ecr get-login-password --region $aws_region --profile dgraph\
| docker login \
    --username AWS \
    --password-stdin $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda
Login Succeeded

> docker push $aws_account_id.dkr.ecr.us-east-1.amazonaws.com/embedding-lambda


```
$ serverless deploy
```



_Note_: In current form, after deployment, your API is protected by an API key. For production deployments, you might want to configure an authorizer.

### Invocation
```
curl --request POST \

--url https://<>.execute-api.us-east-1.amazonaws.com/dev/embedding \
--header 'Content-Type: application/json' --header 'x-api-key: <apikey>' \
--data '{"text":["some sample text","some other text and more info"]}'
```


### Local development

docker build -t python-lambda .
docker run --name embedding -p 8180:8080  -v ./handler.py:/var/task/handler.py   python-lambda

```
curl --request POST \

--url http://localhost:8180/2015-03-31/functions/function/invocations \

--header 'Content-Type: application/json' \

--data '{"body":"{\"text\":[\"some sample text\",\"some other text\"]\n}"}'
```

Note that the payload has a “body” element as a string.

### Bundling dependencies

In case you would like to include 3rd party dependencies, you will need to use a plugin called `serverless-python-requirements`. You can set it up by running the following command:

```bash
serverless plugin install -n serverless-python-requirements
```

Running the above will automatically add `serverless-python-requirements` to `plugins` section in your `serverless.yml` file and add it as a `devDependency` to `package.json` file. The `package.json` file will be automatically created if it doesn't exist beforehand. Now you will be able to add your dependencies to `requirements.txt` file (`Pipfile` and `pyproject.toml` is also supported but requires additional configuration) and they will be automatically injected to Lambda package during build process. For more details about the plugin's configuration, please refer to [official documentation](https://github.com/UnitedIncome/serverless-python-requirements).
