from sentence_transformers import SentenceTransformer
modelPath = "./model"

model = SentenceTransformer('all-MiniLM-L6-v2')
model.save(modelPath)

