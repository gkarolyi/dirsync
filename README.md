# DirSync

A simple web server for directory synchronization.

## Running the Application

### Directly

To run the application directly:

```bash
cd src
go run .
```

This will start the web server on port 8080 (or the port specified in config.json).

### Using Docker

#### Building the Docker Image

To build the Docker image, run the following command from the project root:

```bash
docker build -t dirsync .
```

#### Running the Docker Container

To run the Docker container, use:

```bash
docker run -p 8080:8080 dirsync
```

This will map port 8080 from the container to port 8080 on your host machine.

## Running Tests

To run all tests:

```bash
cd src
go test ./...
```

To run tests without the long-running integration tests:

```bash
cd src
go test -short ./...
```

## Configuration

The application uses a `config.json` file with the following structure:

```json
{
  "sync_interval": 60,
  "sync_pairs": ["source:destination"],
  "port": ":8080"
}
```

- `sync_interval`: Time in seconds between synchronization operations
- `sync_pairs`: Array of source:destination directory pairs to synchronize
- `port`: The port on which the web server listens

## API Endpoints

- `/`: Serves the static web interface
- `/status`: Returns the current synchronization status as JSON