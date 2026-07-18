#!/bin/bash
find . -name "*_test.go" | while read file; do
    sed -i '' 's/models\.Domain/dns.Domain/g' "$file"
    sed -i '' 's/models\.ProviderAccount/dns.ProviderAccount/g' "$file"
    sed -i '' 's/models\.Link/links.Link/g' "$file"
    sed -i '' 's/models\.LinkEvent/links.LinkEvent/g' "$file"
    sed -i '' 's/models\.RoutingRule/links.RoutingRule/g' "$file"
    sed -i '' 's/models\.RoutingRules/links.RoutingRules/g' "$file"
    sed -i '' 's/models\.Mailbox/mailmodels.Mailbox/g' "$file"
    sed -i '' 's/models\.Email/mailmodels.Email/g' "$file"
    sed -i '' 's/models\.Attachment/mailmodels.Attachment/g' "$file"
    sed -i '' 's/models\.SMTPSender/mailmodels.SMTPSender/g' "$file"
    sed -i '' 's/models\.MessageIDHeader/mailmodels.MessageIDHeader/g' "$file"
    
    # Fix import cycle issue if they used mail.* types instead of mailmodels.* types
    sed -i '' 's/mail\.Mailbox/mailmodels.Mailbox/g' "$file"
    sed -i '' 's/mail\.Email/mailmodels.Email/g' "$file"
    sed -i '' 's/mail\.Attachment/mailmodels.Attachment/g' "$file"
    sed -i '' 's/mail\.SMTPSender/mailmodels.SMTPSender/g' "$file"
    sed -i '' 's/mail\.MessageIDHeader/mailmodels.MessageIDHeader/g' "$file"

    # Add correct AutoMigrate
    sed -i '' 's/db.AutoMigrate(models.AllModels()...)/db.AutoMigrate(append(models.AllModels(), \&links.Link{}, \&links.LinkEvent{}, \&dns.Domain{}, \&dns.ProviderAccount{}, \&mailmodels.Mailbox{}, \&mailmodels.Email{}, \&mailmodels.Attachment{}, \&mailmodels.SMTPSender{})...)/g' "$file"
    sed -i '' 's/gdb.AutoMigrate(models.AllModels()...)/gdb.AutoMigrate(append(models.AllModels(), \&links.Link{}, \&links.LinkEvent{}, \&dns.Domain{}, \&dns.ProviderAccount{}, \&mailmodels.Mailbox{}, \&mailmodels.Email{}, \&mailmodels.Attachment{}, \&mailmodels.SMTPSender{})...)/g' "$file"
    sed -i '' 's/db.AutoMigrate(append(models.AllModels(), \&links.Link{}, \&links.LinkEvent{})...)/db.AutoMigrate(append(models.AllModels(), \&links.Link{}, \&links.LinkEvent{}, \&dns.Domain{}, \&dns.ProviderAccount{}, \&mailmodels.Mailbox{}, \&mailmodels.Email{}, \&mailmodels.Attachment{}, \&mailmodels.SMTPSender{})...)/g' "$file"

done
