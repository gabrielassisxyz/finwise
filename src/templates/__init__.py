from jinja2 import Environment, FileSystemLoader
from starlette.templating import Jinja2Templates

env = Environment(loader=FileSystemLoader("src/templates"), autoescape=True, cache_size=0)
templates = Jinja2Templates(env=env)
