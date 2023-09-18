from transformers import AutoModel, AutoTokenizer

def get_model(model):
  """Loads model from Hugginface model hub"""
  try:
    model = AutoModel.from_pretrained(model)
    model.save_pretrained('./model')
  except Exception as e:
    raise(e)

def get_tokenizer(tokenizer):
  """Loads tokenizer from Hugginface model hub"""
  try:
    tokenizer = AutoTokenizer.from_pretrained(tokenizer)
    tokenizer.save_pretrained('./model')
  except Exception as e:
    raise(e)

get_model('sentence-transformers/all-MiniLM-L6-v2')
get_tokenizer('sentence-transformers/all-MiniLM-L6-v2')
