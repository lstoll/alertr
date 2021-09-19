# alertr - simple external monitoring

Really basic tool to run on Heroku to watch an alertmanager/prometheus install

## Deploy

```
heroku container:login
docker buildx build --push --platform linux/amd64 -t registry.heroku.com/<app>/web .
heroku container:release web -a <app>
heroku ps:scale web=0 -a <app> # scheduler only, but things need to be in the web container for it
```
