import json
import os
from sentence_transformers import SentenceTransformer

# change CACHE else we get following error at runtime in AWS Lambda:
# There was a problem when trying to write in your cache folder (/home/sbx_user1051/.cache/huggingface/hub).
os.environ['TRANSFORMERS_CACHE'] = '/var/task'
model = SentenceTransformer('/var/task/model')
def handler(event, context):
    body = json.loads(event['body'])
    sentences = [body[key] for key in body]
    
    embeddings = model.encode(sentences)
    keylist = list(body.keys())
    resp = {keylist[i]:json.dumps(embeddings[i].tolist()) for i in range(len(embeddings))}

    return {"statusCode": 200, "body": json.dumps(resp)}
