from pydantic import BaseSettings, Field


class Settings(BaseSettings):
    app_host: str = Field(default='0.0.0.0', env='APP_HOST')
    app_port: int = Field(default=8080, env='APP_PORT')
    app_debug: bool = Field(default=False, env='APP_DEBUG')
    app_reload: bool = Field(default=False, env='APP_RELOAD')

    class Config:
        env_file = '.env'
        env_file_encoding = 'utf-8'
        case_sensitive = True

settings = Settings()