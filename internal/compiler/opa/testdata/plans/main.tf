terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region                      = "eu-central-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}

resource "aws_ebs_volume" "data" {
  availability_zone = "eu-central-1a"
  size              = 10
  encrypted         = false
}

resource "aws_db_instance" "app" {
  identifier           = "app"
  engine               = "mysql"
  instance_class       = "db.t3.micro"
  allocated_storage    = 20
  username             = "app"
  manage_master_user_password = true
  skip_final_snapshot  = true
  publicly_accessible  = true
  storage_encrypted    = false
}
