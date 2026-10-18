mod cli;
mod clip;
mod config;
mod external;
mod json;
mod query;
mod scaffold;
mod store;

type Result<T> = std::result::Result<T, String>;

fn main() {
    let result = cli::execute();
    if let Err(error) = result {
        cli::err(format!("leo: {error}\n"));
        std::process::exit(1);
    }
}
