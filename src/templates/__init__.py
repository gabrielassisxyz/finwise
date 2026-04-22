import json
from jinja2 import Environment, FileSystemLoader
from starlette.templating import Jinja2Templates

env = Environment(loader=FileSystemLoader("src/templates"), autoescape=True, cache_size=0)
env.filters["fromjson"] = lambda s: json.loads(s)
templates = Jinja2Templates(env=env)
