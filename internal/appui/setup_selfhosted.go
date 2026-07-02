package appui

import (
	"bufio"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"syscall"

	setuptpl "github.com/HexmosTech/git-lrc/setup"
	"golang.org/x/term"
)

func promptSelfHostedLoginFlow(apiURL string, slog *setupLog) (*setupResult, error) {
	setupRequired, err := setuptpl.CheckSelfHostedSetupRequired(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect self-hosted setup status: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)


	if setupRequired {
		fmt.Printf("  %sNo LiveReview users found on this server.%s\n", clr(cYellow), clr(cReset))
		fmt.Printf("  %sCreate the first admin account to continue.%s\n", clr(cDim), clr(cReset))
		fmt.Println()

		admin, err := promptSelfHostedInitialAdmin(reader)
		if err != nil {
			return nil, err
		}
		slog.write("initial self-hosted admin setup for email=%s org=%s", admin.Email, admin.OrgName)
		return setuptpl.ProvisionSelfHostedUser(apiURL, setuptpl.SelfHostedLoginRequest{}, admin, slog.write)
	}

	creds, err := promptSelfHostedCredentials(reader)
	if err != nil {
		return nil, err
	}
	slog.write("self-hosted login for email=%s", creds.Email)
	return setuptpl.ProvisionSelfHostedUser(apiURL, creds, nil, slog.write)
}

func promptSelfHostedCredentials(reader *bufio.Reader) (setuptpl.SelfHostedLoginRequest, error) {
	email, err := promptEmail(reader, "Email")
	if err != nil {
		return setuptpl.SelfHostedLoginRequest{}, err
	}
	password, err := promptSetupPassword(reader, "Password")
	if err != nil {
		return setuptpl.SelfHostedLoginRequest{}, err
	}
	return setuptpl.SelfHostedLoginRequest{
		Email:    email,
		Password: password,
	}, nil
}

func promptSelfHostedInitialAdmin(reader *bufio.Reader) (*setuptpl.SelfHostedSetupAdminRequest, error) {
	email, err := promptEmail(reader, "Admin email")
	if err != nil {
		return nil, err
	}
	password, err := promptSetupPassword(reader, "Admin password")
	if err != nil {
		return nil, err
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	orgName, err := promptSetupLine(reader, "Organization name")
	if err != nil {
		return nil, err
	}
	return &setuptpl.SelfHostedSetupAdminRequest{
		Email:    email,
		Password: password,
		OrgName:  orgName,
	}, nil
}

func promptSetupLine(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("  %s%s:%s ", clr(cBold), label, clr(cReset))
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", strings.ToLower(label), err)
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", strings.ToLower(label))
	}
	return value, nil
}

func promptSetupPassword(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("  %s%s:%s ", clr(cBold), label, clr(cReset))
	if !term.IsTerminal(int(syscall.Stdin)) {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", strings.ToLower(label), err)
		}
		value := strings.TrimSpace(line)
		if value == "" {
			return "", fmt.Errorf("%s cannot be empty", strings.ToLower(label))
		}
		return value, nil
	}

	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", strings.ToLower(label), err)
	}
	value := strings.TrimSpace(string(password))
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", strings.ToLower(label))
	}
	return value, nil
}


func promptEmail(reader *bufio.Reader, label string) (string, error) {
	email, err := promptSetupLine(reader, label)
	if err != nil {
		return "", err
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return "", fmt.Errorf("email must be a valid email address")
	}

	return email, nil
}


