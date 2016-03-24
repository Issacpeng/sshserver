# Plumbing - A key/value engine for Git backend storage

Plumbing is a key/value engine for Git backend storage, can simply accepts data from git client and store repo data in db such as `redis` etc.

## How it works

* Plumbing receive data by SSH protocal and listening on default SSH port `22`.
* Plumbing loads git backend storage lib to store data.
* plumbing receives a git request and auto starts intelligent git server process, you add a git commit and push it to origin repo, it can store in the currect localtion by key/value type. when you clone from origin repo, it can find currect repo data and send it back.

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
```

###pull and push testing example for plumbing
```bash
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
