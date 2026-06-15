#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)

# Repository (GitHub owner/repo)
GH_REPO="nicelic/kwor"

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}Fatal error: ${plain} Please run this script with root privilege \n " && exit 1

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo "Failed to check the system OS, please contact the author!" >&2
    exit 1
fi
echo "The OS release is: $release"

arch() {
    case "$(uname -m)" in
    x86_64 | x64 | amd64) echo 'amd64' ;;
    armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
    *) echo -e "${red}Unsupported CPU architecture! Only amd64 and arm64 are supported.${plain}" && rm -f install.sh && exit 1 ;;
    esac
}

echo "arch: $(arch)"

install_base() {
    case "${release}" in
    centos | almalinux | rocky | oracle)
        yum -y update && yum install -y -q wget curl tar tzdata
        ;;
    fedora)
        dnf -y update && dnf install -y -q wget curl tar tzdata
        ;;
    arch | manjaro | parch)
        pacman -Syu && pacman -Syu --noconfirm wget curl tar tzdata
        ;;
    opensuse-tumbleweed)
        zypper refresh && zypper -q install -y wget curl tar timezone
        ;;
    *)
        apt-get update && apt-get install -y -q wget curl tar tzdata
        ;;
    esac
}

config_after_install() {
    echo -e "${yellow}Migration... ${plain}"
    /usr/local/kwor/kwor migrate

    echo -e "${yellow}Install/update finished! For security it's recommended to modify panel settings ${plain}"
    read -p "Do you want to continue with the modification [y/n]? ": config_confirm
    if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
        echo -e "Enter the ${yellow}panel port${plain} (leave blank for existing/default value):"
        read config_port
        echo -e "Enter the ${yellow}panel path${plain} (leave blank for existing/default value):"
        read config_path

        # Sub configuration
        echo -e "Enter the ${yellow}subscription port${plain} (leave blank for existing/default value):"
        read config_subPort
        echo -e "Enter the ${yellow}subscription path${plain} (leave blank for existing/default value):"
        read config_subPath

        # Set configs
        echo -e "${yellow}Initializing, please wait...${plain}"
        params=""
        [ -z "$config_port" ] || params="$params -port $config_port"
        [ -z "$config_path" ] || params="$params -path $config_path"
        [ -z "$config_subPort" ] || params="$params -subPort $config_subPort"
        [ -z "$config_subPath" ] || params="$params -subPath $config_subPath"
        /usr/local/kwor/kwor setting ${params}

        read -p "Do you want to change admin credentials [y/n]? ": admin_confirm
        if [[ "${admin_confirm}" == "y" || "${admin_confirm}" == "Y" ]]; then
            # First admin credentials
            read -p "Please set up your username:" config_account
            read -p "Please set up your password:" config_password

            # Set credentials
            echo -e "${yellow}Initializing, please wait...${plain}"
            /usr/local/kwor/kwor admin -username ${config_account} -password ${config_password}
        else
            echo -e "${yellow}Your current admin credentials: ${plain}"
            /usr/local/kwor/kwor admin -show
        fi
    else
        echo -e "${red}cancel...${plain}"
        if [[ ! -f "/usr/local/kwor/Promanager_data/db/kwor.db" ]]; then
            local usernameTemp=$(head -c 6 /dev/urandom | base64)
            local passwordTemp=$(head -c 6 /dev/urandom | base64)
            echo -e "this is a fresh installation,will generate random login info for security concerns:"
            echo -e "###############################################"
            echo -e "${green}username:${usernameTemp}${plain}"
            echo -e "${green}password:${passwordTemp}${plain}"
            echo -e "###############################################"
            /usr/local/kwor/kwor admin -username ${usernameTemp} -password ${passwordTemp}
        else
            echo -e "${red} this is your upgrade,will keep old settings${plain}"
        fi
    fi
}

prepare_services() {
    systemctl daemon-reload
}

install_kwor() {
    cd /tmp/

    if [ $# == 0 ]; then
        last_version=$(curl -Ls "https://api.github.com/repos/${GH_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$last_version" ]]; then
            echo -e "${red}Failed to fetch kwor version, it maybe due to Github API restrictions, please try it later${plain}"
            exit 1
        fi
        echo -e "Got kwor latest version: ${last_version}, beginning the installation..."
        wget -N --no-check-certificate -O /tmp/kwor-linux-$(arch).tar.gz https://github.com/${GH_REPO}/releases/download/${last_version}/kwor-linux-$(arch).tar.gz
        if [[ $? -ne 0 ]]; then
            echo -e "${red}Downloading kwor failed, please be sure that your server can access Github ${plain}"
            exit 1
        fi
    else
        last_version=$1
        url="https://github.com/${GH_REPO}/releases/download/${last_version}/kwor-linux-$(arch).tar.gz"
        echo -e "Beginning the install kwor v$1"
        wget -N --no-check-certificate -O /tmp/kwor-linux-$(arch).tar.gz ${url}
        if [[ $? -ne 0 ]]; then
            echo -e "${red}download kwor v$1 failed,please check the version exists${plain}"
            exit 1
        fi
    fi

    if [[ -e /usr/local/kwor/ ]]; then
        systemctl stop kwor 2>/dev/null
    fi

    tar zxvf kwor-linux-$(arch).tar.gz
    rm kwor-linux-$(arch).tar.gz -f

    chmod +x kwor/kwor
    mkdir -p /usr/local/kwor
    cp -f kwor/kwor /usr/local/kwor/
    cp -f kwor/*.service /etc/systemd/system/
    rm -rf kwor

    config_after_install
    prepare_services

    systemctl enable kwor --now

    echo -e "${green}kwor ${last_version}${plain} installation finished, it is up and running now..."
    echo -e "You may access the Panel with following URL(s):${green}"
    /usr/local/kwor/kwor uri
    echo -e "${plain}"
}

echo -e "${green}Executing...${plain}"
install_base
install_kwor $1
