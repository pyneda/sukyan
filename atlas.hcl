data "external_schema" "gorm" {
  program = [
    "go", "run", "-mod=mod",
    "./db/atlas"
  ]
}

env "gorm" {
  src = data.external_schema.gorm.url
  url = getenv("POSTGRES_DSN")
  dev = "docker://postgres/15/dev?search_path=public"
  migration {
    dir = "file://db/migrations"
  }
  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
