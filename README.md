# Plumbing - A key/value engine for Git backend storage

Plumbing is a key/value engine for Git backend storage, can simply accepts data from git command, and loads git backend storage lib to storage data in key/value db such as `redis` etc.

## How it works

* Plumbing starts intelligent git server process and listening on default SSH protocol port:22.
* Plumbing loads git backend storage lib to store data.
* Plumbing receives a git request, when you add a git commit and push it to origin repo, can loads git backend storage lib to store in the currect localtion in key/value db. when you clone from origin repo, can find currect data and back to local path.

## Usage

### Compile

Compile is as simple as:

```bash
# download repo
$ git clone https://github.com/containerops/plumbing
$ cd plumbing
# Download dependencies and compile the project
$ go get && go build
# Run it! You can set SSH_PORT to customize the SSH port it serves on.
$ ./plumbing
# copy private key to your own local path
$ cp -f ssh/id_rsa ~/.ssh


###test example for plumbing

# create a empty repo on server 
$ mkdir -p myrepo/testuser/testreponame.git
$ cd myrepo/testuser/testreponame.git
$ git init --bare

# push repo to plumbing
# clone project from github and `cd` into its directory, add your remote point to plumbing
$ git clone https://github.com/docker/docker.git
$ git remote add test git@localhost:testuser/testreponame.git
$ git push test master

# clone project from plumbing
$ git clone git@localhost:testuser/testreponame.git
```
