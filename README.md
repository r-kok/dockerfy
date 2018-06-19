![dockerfyme](https://github.com/markriggins/dockerfy/blob/master/dockerfyme.png)
dockerfy -- Utility to initialize docker containers
===================================================
**Dockerfy** is a utility program to initialize and control container applications, and also provide some
missing OS functionality (such as an init process, and reaping zombies etc.)

### Open Source
Dockerfy is an open-source project under Apache 2.0 license.  This clone is up to date, but the official source has now moved to [SocialCodeInc/dockerfy](https://github.com/SocialCodeInc/dockerfy/)

##Key Features

1. Overlays of alternative content at runtime
2. Templates for configuration and content
3. Environment Variable substitutions into templates and overlays
4. Secrets injected into configuration files (without leaking them to the environment)
5. Waiting for dependencies (any server and port) to become available before the primary command starts
6. Tailing log files to the container's stdout and/or stderr
7. Running commands before the primary command begins
8. Starting Services -- and shutting down the container if they fail
9. Propagating signals to child processes
10. Reaping Zombie (defunct) processes
11. Running services and commands under various user accounts

This small program packs in a lot of features, and can be added to almost any Docker image to make it easy to pre-initialize the container at runtime.
Pre-built binaries are available on our [Releases](https://github.com/SocialCodeInc/dockerfy/releases) page.


## Dockerfile Example

    FROM socialcode/nginx-with-dockerfy

    ENTRYPOINT [ "dockerfy",                                                                            \
                    "--secrets-files", "/secrets/secrets.env",                                          \
                    "--overlay", "/app/overlays/{{ .Env.DEPLOYMENT_ENV }}/html/:/usr/share/nginx/html",         \
                    "--template", "/app/nginx.conf.tmpl:/etc/nginx/nginx.conf",                         \
                    "--wait", 'tcp://{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}', "--timeout", "60s",  \
                    "--run", '/app/bin/migrate_lock', "--server='{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}'",  "--", \
                    "--start", "/app/bin/cache-cleaner-daemon", "-p", "{{ .Secret.DB_PASSWORD }}", "--",\
                    "--reap",                                                                           \
                    "--user", "nobody",                                                                 \
                  	"nginx",  "-g",  "daemon off;" ]

## equivalent docker-compose.yml Example

    nginx:
      image: socialcode/nginx-with-dockerfy

      volumes:
        - /secrets:/secrets

      environment:
        - SECRETS_FILES=/secrets/secrets.env

      entrypoint:
        - dockerfy

      command: [
        "--overlay", "/app/overlays/{{ .Env.DEPLOYMENT_ENV }}/html/:/usr/share/nginx/html",
        "--template", "/app/nginx.conf.tmpl:/etc/nginx/nginx.conf",
        "--wait", "tcp://{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}", "--timeout", "60s",
        "--wait", "tcp://{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}", "--timeout", "60s",
        "--run", "/app/bin/migrate_lock", "--server='{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}'",  "--",
        "--start", "/app/bin/cache-cleaner-daemon", "-p", '{{ .Secret.DB_PASSWORD }}', "--",
        "--reap",
        "--user", "nobody",
        '--', 'nginx', '-g', 'daemon off;' ]



The above example will run the nginx program inside a docker container, but **before nginx starts**, **dockerfy** will:

1. **Sparsely Overlay** files from the application's /app/overlays directory tree from /app/overlays/${DEPLOYMENT_ENV}/html **onto** /usr/share/nginx/html.  For example, the robots.txt file might be restrictive in the "staging" deployment environment, but relaxed in "production", so the application can maintain separate copies of robots.txt for each deployment environment: /app/overlays/staging/robots.txt, and /app/overlays/production/robots.txt.   Overlays add or replace files similar to `cp -R` without affecting other existing files in the target directory.
2. **Load secret settings** from a file a /secrets/secrets.env, that become available for use in templates as {{ .Secret.**VARNAME** }}
3. **Evaluate the nginx.conf.tmpl template**. This template uses the powerful go language templating features to substitute environment variables and secret settings directly into the nginx.conf file. (Which is handy since nginx doesn't read the environment itself.)  Every occurance of {{ .Env.**VARNAME** }} will be replaced with the value of $VARNAME, and every {{ .Secret.**VARNAME** }} will be replaced with the secret value of VARNAME.
4. **Wait** for the http://{{ .Env.MYSQLSERVER }} server to start accepting requests on port {{ .Env.MYSQLPORT }} for up to 60 seconds
5. **Run migrate_lock** a program to perform a Django/MySql database migration to update the database schema, and wait for it to finish. If **migrate_lock** fails, then dockerfy will exit with migrate_lock's exit code, and the primary command **nginx** will never start.
6. **Start the cache-cleaner-daemon**, which will run in the background presumably cleaning up stale cache files while nginx runs.  If for any reason the cache-cleaner-daemon exits, then dockerfy will also exit with the cache-cleaner-daemon's exit code.
7. **Start Reaping Zombie processes** under a separate goroutine in case the cache-cleaner-deamon loses track of its child processes.
8. **Run nginx** with its customized nginx.conf file and html as user `nobody`
9. **Propagate Signals** to all processes, so the container can exit cleanly on SIGHUP or SIGINT
10. **Monitor Processes** and exit if nginx or the cache-cleaner-daemon dies
11. **Exit** with the primary command's exit_code if the primary command finishes.


This all assumes that the /secrets volume was mounted and the environment variables $MYSQLSERVER, $MYSQLPORT
and $DEPLOYMENT_ENV were set when the container started.  Note that **dockerfy** expands the environment variables in its arguments, since the ENTRYPOINT [] form in Dockerfiles does not, replacing all {{ .Env.VARNAME }} and {{ .Secret.VARNAME }} occurances with their values from the environment or secrets files.

Note that the unexpanded argument '{{ .Secret.DB_PASSWORD }}', would be visible in `ps -ef` output, not the actual password

Note that ${VAR_NAME}'s are NOT expanded by dockerfy because docker-compose and ecs-cli also expand environment variables inside yaml files.  The {{ .Env.VAR_NAME }} form passes through easily, as long as it is inside a singly-quoted string

The "--" argument is used to signify the end of arguments for a --start or --run command.


# Typical Use-Case
The typical use case for **dockerfy** is when you have an
application that:

1. Relies strictly on configuration files to initialize itself. For example, ningx does not use environment variables directly inside nginx.conf files
2. Needs to wait for some other service to become available.  For example, in a docker-compose.yml application with a webserver and a database, the webserver may need to wait for the the database to initialize itself at start listening for logins before the webserver starts accepting requests, or tries to connect to the database.
3. Needs to run some initialization before the real application starts.  For example, applications that rely on a dedicated database may need to run a migrations script to update the database
4. Needs a companion service to run in the background, such as uwsgi, or a cleanup daeamon to purge caches.
5. Is a long-lived Container that runs a complex application.  For example, if the long-lived application forks a lot of child processes that forget to wait for their own children, then OS resources can consumed by defunct (zombie) processes, eventually filling the process table.
6. Needs Passwords or other Secrets.  For example, a Django server might need to login to a database, but passing the password through the environment or a run-time flag is susceptible to accidental leakage.

Another use case is when the application logs to specific files on the filesystem and not stdout
or stderr. This makes it difficult to troubleshoot the container using the `docker logs` command.
For example, nginx will log to `/var/log/nginx/access.log` and
`/var/log/nginx/error.log` by default. While you can work around this for nginx by replacing the access.log file with a symbolic link to /dev/stdout,  **dockerfy** offers a generic solution allowing you to specify which logs files should
be tailed and where they should be sent.

## Customizing Startup and Application Configuration

### Sparse Overlays
Overlays are used provide alternative versions of entire files for various deployment environments (or other reasons).  **[Unlike mounted volumes, overlays do not hide the existing directories and files, they copy the altenative content ONTO the existing content, replacing only what is necessary]**.  This comes in handy for *if-less* languages like CSS, robots.txt, and for icons and images that might need to change depending on the deployment environment.  To use overlays, the application can create a directory tree someplace, perhaps at ./overlays with subdirectories for the various deployment environents like this:

    ./
    ├── _overlays
    │	 ├── _common
    │	 │   └── html
    │	 │       └── robots.txt
    │	 ├── prod
    │	 │   └── html
    │	 │       └── robots.txt
    │	 └── staging
    │   	└── html
    │        	└── index.html
    └── _root
         └── etc
             └── nginx
                 └── nginx.conf

The entire ./_overlays and ./_root files must be COPY'd into the Docker image (usually along with the application itself):

	COPY . /app

Then the desired alternative for the files can be chosen at runtime use the --overlay *src:dest* option

	$ dockerfy --overlay /app/_overlays/_commmon/html:/usr/share/nginx/ \
            --overlay /app/_overlays/{{ .Env.DEPLOYMENT_ENV }}/html:/usr/share/nginx/ \
            nginx -g "daemon off;"

If the source path ends with a /, then all subdirectories underneath it will be copied.  This allows copying onto the root file system as the destination; so you can `-overlay /app/_root/:/` to copy files such as /app/_root/etc/nginx/nginx.conf --> /etc/nginx/nginx.conf.   This is handy if you need to drop a lot of files into various exact locations

Overlay sources that do not exist are simply skipped.  The allows you to specify potential sources of content that may or may not exist in the running container.  In the above example if $DEPLOYMENT_ENV environment variable is set to 'local' then the second overlaw will be skipped if there is no corresponding /app/overlays/local source directory, and the container will run with the '_common' html content.

#### Loading Secret Settings
Secrets can be loaded from files by using the `--secrets-files` option or the $SECRETS_FILES environment variable.   The secrets files ending with `.env` must contain simple NAME=VALUE lines, following bash shell conventions for definitions and comments. Leading and trailing quotes will be trimmed from the value.  Secrets files ending with `.json` will be loaded as JSON, and must be `a simple single-level dictionary of strings`

    #
    # These are our secrets
    #
    PROXY_PASSWORD="a2luZzppc25ha2Vk"

or secrets.json (which must be **a simple single-level dictionary of strings**)

    {
      "PROXY_PASSWORD": "a2luZzppc25ha2Vk"
    }

Secrets can be injected into configuration files by using [Secrets in Templates](https://github.com/markriggins/dockerfy#secrets-in-templates).

You can specify multiple secrets files by using a colon to separate the paths, or by using the `--secrets-files` option multiple times. The files will be processed in the order that they were listed. Values from the later files overwrite earlier values if there are duplicates.

For convenience, all secrets files are combined into ~/.secrets/combined_secrets.json inside the ephemeral running
container for each `--user` account in the users home directory so the program running as the user will have permission to read the values and so JavaScript, Python and Go programs can load the secrets programatically from a single file.  The combined secrets file location is exported as $SECRETS_FILE into the running --start, --run and primary command's environments.

#### Loading Secret Settings from AWS Systems Manager Parameter Store
**Dockerfy** can also load secrets stored in the AWS Systems Manager [Parameter Store](https://aws.amazon.com/ec2/systems-manager/parameter-store/).
If you specify an expression like `{{ .AWS_Secret.**VARNAME** }}` in a template then dockerfy will try to fetch the parameter from the AWS Parameter store.
If the parameter cannot be found (due to lack or permission or because it does not exist) dockerfy falls back to using the value of a corresponding ENVIRONMENT value.
**Dockerfy** retrieves the decoded values of at most 10 parameters. 

You can store multiple parameters in the Parameter Store with different prefixes. For example, for production and test:

    PROD_DB_PASSWORD = xxx
    TEST_DB_PASSWORD = yyy

To select a specific parameter that matches the prefix specify option `--aws-secret-prefix PROD_`. 
You can use `{{ .AWS_Secret.**DB_PASSWORD** }}` in your template, thus without the prefix.


##### Security Concerns
1. **Reading secrets from files** -- Dockerfy only passes secrets to programs via configuration files to prevent leakage. Secrets could be passed to programs via the environment, but programs use the environment in unpredictable ways, such as logging, or perhaps even dumping their state back to the browser.
2. **Installing Secrets** -- The recommended way to install secrets in production environments is to save them to a tightly protected place on the host and then mount that directory into running docker containers that need secrets. Yes, this is host-level security, but at this point in time, if the host running the docker daemon is not secure, then security has already been compromised.
3. **Tokens** -- Tokens that are revokable, or can be configured to expire, are much safer to use as secrets than long-lived passwords.
4. **Hashed and Salted** --  If passwords must be used, they should be stored only in a salted, and hashed form, never as plain-text or base64 or simply encrypted.  Without salt, passwords can be broken with a dictionary attack

#### Executing Templates
This `--template src:dest` option uses the powerful [go language templating](http://golang.org/pkg/text/template/) capability to substitute environment variables and secret settings directly into the template source and writes the result onto the template destination.

#####Simple Template Substitutions -- an nginx.conf.tmpl

	server {
      location / {
        proxy_pass {{ .Env.PROXY_PASS_URL }};
        proxy_set_header Authorization "Basic {{ .Secret.PROXY_PASSWORD }}";

        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_redirect {{ .Env.PROXY_PASS_URL }} $host;
      }
    }

In the above example, all occurances of the string  {{ .Env.PROXY_PASS_URL }} will be replaced with the value of $PROXY_PASS_URL from the container's environment, and {{ .Secret.PROXY_PASSWORD }} will be replaced with its value, giving the result:

	server {
      location / {
        proxy_pass http://myserver.com;
        proxy_set_header Authorization "Basic a2luZzppc25ha2Vk";

        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_redirect http://myserver.com $host;
      }
    }


Note: $host and $remote_addr are Nginx variables that are set on a per-request basis NOT from the environment.

##### Advanced Templates
But go's templates offer advanced features such as if-statements and comments.  The example below will add a `location /` block to setup proxy_pass only if the environment variable $PROXY_PASS_URL is set.

	server {
    {{/* only set up proxy_pass if PROXY_PASS_URL is set in the environment */}}
    {{ if .Env.PROXY_PASS_URL }}
      location / {
        proxy_pass {{ .Env.PROXY_PASS_URL }};

        {{ if .Secret.PROXY_PASSWORD }}
        proxy_set_header Authorization "Basic {{ .Secret.PROXY_PASSWORD }}";
        {{ end }}


        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_redirect {{ .Env.PROXY_PASS_URL }} $host;
      }
    {{ end }}
    }

If your source language uses {{ }} for some other purpose, you can avoid the conflict by using the `--delims` option to specify alternative delimiters such as "<%:%>"

##### Built-in Template Functions
There are a few built in functions as well:

  * `default $var $default` - Returns a default value for one that does not exist. `{{ default .Env.VERSION "0.1.2" }}`
  * `contains $map $key` - Returns true if a string is within another string
  * `exists $path` - Determines if a file path exists or not. `{{ exists "/etc/default/myapp" }}`
  * `split $string $sep` - Splits a string into an array using a separator string. Alias for [`strings.Split`][go.string.Split]. `{{ split .Env.PATH ":" }}`
  * `replace $string $old $new $count` - Replaces all occurrences of a string within another string. Alias for [`strings.Replace`][go.string.Replace]. `{{ replace .Env.PATH ":" }}`
  * `parseUrl $url` - Parses a URL into it's [protocol, scheme, host, etc. parts][go.url.URL]. Alias for [`url.Parse`][go.url.Parse]
  * `atoi $value` - Parses a string $value into an int. `{{ if (gt (atoi .Env.NUM_THREADS) 1) }}`
  * `add $arg1 $arg` - Performs integer addition. `{{ add (atoi .Env.SHARD_NUM) -1 }}`
  * `sequence "2" "5"` - Returns an array with the values from first to last.  In this case, [ "2", "3", "4", "5"], that can serve as the basis for iteration.
  * `contact "ab" "c" "d"` - Returns the concatonation of its arguments "abcd".
  * `getenv "VAR1"` - Returns the value of the environment variable $VAR1

##### Template Iteration
Golang templates offer a unique method of iteration that is somewhat obtuse to say the least, so a worked example may be best to show you how it works.

    example.tmpl:
        {{range $i, $v := sequence "5" "8"}}
            the value of sequence[{{$i}}] is {{$v}}
        {{end}}

The above template uses the `sequence` function to create a list of numbers ["5", "6", "7", "8"] to be used at the argument for the GoLang template `range` function. Everything between the `{{range ..}}` and `{{end}}` will be evaluated `once per sequence value`, expanding the template to :

     the value of sequence[0] is 5
     the value of sequence[1] is 6
     the value of sequence[2] is 7
     the value of sequence[3] is 8

Combining `range` with `split` can be used to print `0=a 1=b 2=c`

     {{range $i, $v := split "a,b,c" ","}}
       {{$i}}={{$v}}
     {{end}}

Combining this with `getenv` command allows us control the iteration by environment variables to print the numbers 1 to $HOW_MANY

     env-example.tmpl:
         {{ $how_many := getenv "HOW_MANY"}}
         {{range $i, $v := sequence "1" $how_many}}
           {{$v}}
         {{end}}

Note, GoLang tempaltes also support piping, which you can use pass the name of an environment variable to `getenv` so variable names can be made on the fly, which you could use to read some environment variables named $V_1, $V_2,...

    {{range $i, $v := sequence "1" "3"}}
        {{ $val := printf "V_%s" $v | getenv }}
        environment variable V_{{$v}} == "{{$val}}"
    {{end}}

To produce the following, (assuming that the enviroment variables have been exported with values "one", "two", and "three"

    environment variable V_1 == "one"
    environment variable V_2 == "two"
    environment variable V_3 == "three"

Which might come in handy for setting up nginx downstream servers, or whatever you might need.  Go crazy, but try not to hurt yourself. :)


##### Secrets During Development
If you're running in locally development mode and mounting the current directory `-v $PWD:/app` in your docker container, **please resist the temptation of storing your secrets in files under GIT control**.   Instead, we recommend creating a ~/.secrets sub-directory in your $HOME directory to store secrets.

1. Create a ~/.secrets directory with permissions 700
2. Create a separate secrets file for each application and deployment environment with permissions 600.  Having separate files allows you to avoid lumping all your secrets for all applications and deployment environments into a single file.

	~/.secrets/my-application--production.env
	~/.secrets/my-application--staging.env

3. Mount the ~/secrets directory at /secrets when you run your container
4. Export SECRETS_FILES=/secrets/my-application--$DEPLOYMENT_ENV.env or use the `--secrets-files` command line option to tell dockerfy where to find the secrets file(s).

**Example:**

		$ mkdir ~/.secrets
		$ chmod 700 ~/.secrets
		$ echo "PASSWORD='top secret'" > ~/.secrets/my-application--staging.env
		$ chmod 600 ~/.secrets/*

		$ docker run -v $HOME/.secrets:/secrets --entrypoint dockerfy socialcode/nginx-with-dockerfy \
		    --secrets-files /secrets/my-application--staging.env -- echo 'The password is "{{.Secret.PASSWORD}}"'

Will print `The password is "top secret"`

While developing, avoid evaluating templates onto locations within your mounted worktree accidentally.  **The expanded results might contain secrets!!** and even worse, if you forget to add them to your .gitignore file, then **your secrets could wind up on github.com!!**  Instead, write them to /etc/ or some other place inside the running container that will be forgotten when the container exits.


### Waiting for other dependencies

It is common when using tools like [Docker Compose](https://docs.docker.com/compose/) to depend on services in other linked containers, however oftentimes relying on [links](https://docs.docker.com/compose/compose-file/#links) is not enough - whilst the container itself may have _started_, the _service(s)_ within it may not yet be ready - resulting in shell script hacks to work around race conditions.

**Dockerfy** gives you the ability to wait for services on a specified protocol (`tcp`, `tcp4`, `tcp6`, `http`, and `https`) before running commands, starting services, or starting your application.

NOTE: MySql server is not an HTTP server, so use the tcp protocol instead of http

	$ dockerfy --wait 'tcp://{{ .Env.MYSQLSERVER }}:{{ .Env.MYSQLPORT }}' --timeout 120s ...

You can specify multiple dependancies by repeating the --wait flag.  If the dependancies fail to become available before the timeout (which defaults to 10 seconds), then dockery will exit, and your primary command will not be run.

NOTE: If for some reason dockerfy cannot resolve the DNS names for links try exporting GODEBUG=netdns=cgo to force dockerfy to use cgo for DNS resolution.  This is a known issue on Docker version 1.12.0-rc3, build 91e29e8, experimental for OS X.

### Running Commands
The `--run` option gives you the opportunity to run commands **after** the overlays, secrets and templates have been processed, but **before** the primary program begins.  You can run anything you like, even bash scripts like this:

	$ dockerfy  \
		--run rm -rf /tmp/* -- \
		--run bash -c "sleep 10, echo 'Lets get started now'" -- \
        nginx -g "daemon off;"

All options up to but not including the '--' will be passed to the command.  You can run as many commands as you like, they will be run in the same order as how they were provided on the command line, and all commands must finish **successfully** or **dockerfy** will exit and your primary program will never run.


### Starting Services
The `--start` option gives you the opportunity to start a commands as a service **after** the overlays, secrets and templates have been processed, and all --run commands have completed,  but **before** the primary program begins.  You can start anything you like as a service, even bash scripts like this:

	$ dockerfy  \
		--start "bash -c "while true; do rm -rf /tmp/cache/*; sleep 3600; done" -- \
		nginx -g "daemon off;"

All options up to but not including the '--' will be passed to the command.  You can start as many services as you like, they will all be started in the same order as how they were provided on the command line, and all commands must continue **successfully** or **dockerfy** will
stop your primary command and exit, and the container will stop.

#### Debugging Dockerfy
If dockerfy isn't behaving as you expect, then try the `--verbose` or `--debug` flags to view more detailed output, including details about how `--run` and `--start` commands are processed.

NOTE: The `--debug` flag is discouraged in production because it will leak the names of secrets variables to the logs

### Switching User Accounts
The `--user` option gives you the ability specify which user accounts with which to run commands or start services.  The `--user` flag takes either a username or UID as its argument, and affects all subsequent commands.

  $ dockerfy \
    --user mark --run id -F -- \
    --user bob  --run id -F -- \
    --user 0    --run id -F -- \
    id -a

The above command will first run the `id -F` command as user "mark", which will print mark's full name "Mark Riggins".
Then it will print bob's full name.  Next it will print the full name of the account with user id 0, which happens to be "root".  Finally the primary command `id` will run with as the user account of the `last` invokation of the `--user` option, giving us the full id information for the root account.

The **dockerfy** command itself will continue to run as the root user so it will have permission to reap zombies and monitor and signal any services that were started.

NOTE: The **--user** flag only works for VALID user names that have already been defined before dockerfy runs.  Additional user accounts should be created by the docker RUN directive when the image is built.

### Reaping Zombies
Long-lived containers should with services use the `--reap` option to clean up any zombie processes that might arise if a service fails to wait for its child processes to die.  Otherwise, eventually the process table can fill up and your container will become unresponsive.  Normally the init daemon would do this important task, but docker containers do not have an init daemon, so **dockerfy** will assume the responsibility.

Note that in order for this work fully, **dockerfy** should be the primary processes with pid 1. Orphaned child processes are all adopted by the primary process, which allows its to wait for them and collect their exit codes and signals, thus clearing the defunct process table entry.   This means that **dockerfy** must be the FIRST command in your ENTRYPOINT or CMD inside your Dockerfile

### Propagating Signals
**Dockerfy** passes SIGHUP, SIGINT, SIGQUIT, SIGTERM and SIGKILL to all commands and services, giving them a brief chance to respond, and then kills them and exits.  This allows your container to exit gracefully, and completely shut down services, and not hang when it us run in interactive mode via `docker run -it ...` when you type ^C

### Tailing Log Files
Some programs (like nginx) insist on writing their logs to log files instead of stdout and stderr.  Although nginx can be tricked into doing the desired thing by replacing the default log files with symbolic links to /dev/stdout and /dev/stderr, we really don't know how every program out there does its logging, so **dockerfy** gives you to option of tailing as many log files as you wish to stdout and stderr via the --stdout and --stderr flags.

	$ dockerfy --stdout info.log --stdout perf.log


If `inotify` does not work in you container, you use the `--log-poll` option to tell **dockerfy** to poll for file changes.



## Installation

Download the latest version in your container:
[releases](https://github.com/SocialCodeInc/dockerfy/releases)

For Linux Amd64 Systems:

```
RUN wget https://github.com/SocialCodeInc/dockerfy/files/498659/dockerfy-linux-amd64-1.0.0.tar.gz; \
	tar -C /usr/local/bin -xzvf dockerfy-linux-amd64*.gz; \
	rm dockerfy-linux-amd64*.gz;
```
But of course, use the latest release and a binary that suits your system architecture.


##Inspiration and Open Source Usage
Dockerfy is based on the work of others, relying heavily on jwilder's `dockerize` program for how to wait for processes to finish, and how to tail log files ( [ see jwilder/dockerize](https://github.com/jwilder/dockerize) and [A Simple Way To Dockerize Applications](http://jasonwilder.com/blog/2014/10/13/a-simple-way-to-dockerize-applications/) ), but `dockerfy` expands the features of GoLanguage templates and adds many new features such as secrets injection, overlays, and commands that run before the primary command starts and the command-line syntax for specifying arguments to the commands that should start or run


TODO:
    convert everything to camelCase

