* installare aws cli 2

https://docs.aws.amazon.com/it_it/cli/latest/userguide/install-cliv2.html

* configurare le credenziali

https://docs.aws.amazon.com/it_it/cli/latest/userguide/cli-configure-files.html

* definire profili

```
[default]
region = eu-west-1

[profile lab]
source_profile = default
region = eu-west-1

[profile test]
role_arn = arn:aws:iam::***
source_profile = default
region = eu-west-1

[profile prod]
role_arn = arn:aws:iam::***
source_profile = default
region = eu-west-1
```
# ash
